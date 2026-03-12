package scheduler

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/41490/ccclaw/internal/logging"
)

const (
	managedLogArchiveHeader = "# ccclaw scheduler logs archive"
	managedLogArchivePrefix = "ccclaw_scheduler_"
)

var (
	managedLogArchiveNamePattern = regexp.MustCompile(`^ccclaw_scheduler_\d{8}_\d{6}_[a-z0-9-]+\.log(?:\.gz)?$`)
	legacyLogArchiveNamePattern  = regexp.MustCompile(`^\d{6}_\d{6}_[a-z0-9-]+\.log(?:\.gz)?$`)
)

type LogArchivePolicy struct {
	RetentionDays int
	MaxFiles      int
	Compress      bool
}

type LogsOptions struct {
	Scope         string
	Follow        bool
	Since         string
	Lines         int
	Level         string
	ArchivePath   string
	ArchivePolicy LogArchivePolicy
}

type managedArchiveFile struct {
	Path       string
	Name       string
	ModTime    time.Time
	Compressed bool
}

func StreamLogs(ctx context.Context, options LogsOptions, stdout, stderr io.Writer) error {
	if _, err := exec.LookPath("journalctl"); err != nil {
		return fmt.Errorf("未找到 journalctl: %w", err)
	}
	units, err := managedLogUnits(options.Scope)
	if err != nil {
		return err
	}
	level, err := normalizeJournalLevel(options.Level)
	if err != nil {
		return err
	}
	args := []string{"--user", "--no-pager"}
	if options.Lines <= 0 {
		options.Lines = 50
	}
	args = append(args, "-n", fmt.Sprintf("%d", options.Lines))
	if level != "" {
		args = append(args, "-p", level)
	}
	if strings.TrimSpace(options.Since) != "" {
		args = append(args, "--since", options.Since)
	}
	if options.Follow {
		args = append(args, "-f")
	}
	for _, unit := range units {
		args = append(args, "-u", unit)
	}
	logStdout := stdout
	logStderr := stderr
	var archive *os.File
	if strings.TrimSpace(options.ArchivePath) != "" {
		archive, err = openLogArchive(options)
		if err != nil {
			return err
		}
		logStdout = io.MultiWriter(stdout, archive)
		logStderr = io.MultiWriter(stderr, archive)
	}
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	cmd.Stdout = logStdout
	cmd.Stderr = logStderr
	cmd.Stdin = strings.NewReader("")
	runErr := cmd.Run()
	if archive != nil {
		if closeErr := archive.Close(); closeErr != nil {
			return fmt.Errorf("关闭日志归档文件失败: %w", closeErr)
		}
	}
	if runErr != nil {
		return fmt.Errorf("执行 journalctl 失败: %w", runErr)
	}
	if strings.TrimSpace(options.ArchivePath) != "" {
		if err := applyLogArchivePolicy(filepath.Dir(options.ArchivePath), options.ArchivePath, options.ArchivePolicy); err != nil {
			return fmt.Errorf(
				"日志已归档到 %s，但保留策略执行失败: %w；请检查目录权限，或暂时将 scheduler.logs.compress 设为 false",
				options.ArchivePath,
				err,
			)
		}
	}
	return nil
}

func BuildLogArchivePath(baseDir, scope string, now time.Time) string {
	scope = sanitizeLogScope(scope)
	filename := fmt.Sprintf("%s%s_%s.log", managedLogArchivePrefix, now.Format("20060102_150405"), scope)
	return filepath.Join(baseDir, filename)
}

func normalizeJournalLevel(level string) (string, error) {
	return logging.JournalPriority(level)
}

func openLogArchive(options LogsOptions) (*os.File, error) {
	path := strings.TrimSpace(options.ArchivePath)
	if path == "" {
		return nil, fmt.Errorf("归档路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("创建日志归档目录失败: %w", err)
	}
	handle, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("打开日志归档文件失败: %w", err)
	}
	header := strings.Builder{}
	scope := sanitizeLogScope(options.Scope)
	header.WriteString(managedLogArchiveHeader + "\n")
	header.WriteString(fmt.Sprintf("# archived_at: %s\n", time.Now().Format(time.RFC3339)))
	header.WriteString(fmt.Sprintf("# scope: %s\n", scope))
	header.WriteString("# managed_by: ccclaw\n")
	if level := strings.TrimSpace(options.Level); level != "" {
		header.WriteString(fmt.Sprintf("# level: %s\n", strings.ToLower(level)))
	}
	if since := strings.TrimSpace(options.Since); since != "" {
		header.WriteString(fmt.Sprintf("# since: %s\n", since))
	}
	header.WriteString("\n")
	if _, err := io.WriteString(handle, header.String()); err != nil {
		_ = handle.Close()
		return nil, fmt.Errorf("写入日志归档头失败: %w", err)
	}
	return handle, nil
}

func sanitizeLogScope(scope string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		return "all"
	}
	var cleaned strings.Builder
	for _, r := range scope {
		switch {
		case r >= 'a' && r <= 'z':
			cleaned.WriteRune(r)
		case r >= '0' && r <= '9':
			cleaned.WriteRune(r)
		case r == '-':
			cleaned.WriteRune(r)
		}
	}
	if cleaned.Len() == 0 {
		return "all"
	}
	return cleaned.String()
}

func applyLogArchivePolicy(baseDir, currentPath string, policy LogArchivePolicy) error {
	if strings.TrimSpace(baseDir) == "" {
		return nil
	}

	files, err := listManagedArchiveFiles(baseDir)
	if err != nil {
		return err
	}
	if policy.Compress {
		for _, file := range files {
			if sameArchivePath(file.Path, currentPath) || file.Compressed {
				continue
			}
			if _, err := gzipArchiveFile(file.Path, file.ModTime); err != nil {
				return fmt.Errorf("压缩 %s 失败: %w", file.Name, err)
			}
		}
		files, err = listManagedArchiveFiles(baseDir)
		if err != nil {
			return err
		}
	}
	if policy.RetentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -policy.RetentionDays)
		for _, file := range files {
			if !file.ModTime.Before(cutoff) {
				continue
			}
			if err := os.Remove(file.Path); err != nil {
				return fmt.Errorf("删除过期归档 %s 失败: %w", file.Name, err)
			}
		}
		files, err = listManagedArchiveFiles(baseDir)
		if err != nil {
			return err
		}
	}
	if policy.MaxFiles > 0 && len(files) > policy.MaxFiles {
		sort.Slice(files, func(i, j int) bool {
			if files[i].ModTime.Equal(files[j].ModTime) {
				return files[i].Name > files[j].Name
			}
			return files[i].ModTime.After(files[j].ModTime)
		})
		for _, file := range files[policy.MaxFiles:] {
			if err := os.Remove(file.Path); err != nil {
				return fmt.Errorf("删除超额归档 %s 失败: %w", file.Name, err)
			}
		}
	}
	return nil
}

func listManagedArchiveFiles(baseDir string) ([]managedArchiveFile, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取归档目录失败: %w", err)
	}
	files := make([]managedArchiveFile, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("读取归档文件信息失败 %s: %w", entry.Name(), err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		path := filepath.Join(baseDir, entry.Name())
		managed, compressed, err := isManagedArchiveFile(path)
		if err != nil {
			return nil, err
		}
		if !managed {
			continue
		}
		files = append(files, managedArchiveFile{
			Path:       path,
			Name:       entry.Name(),
			ModTime:    info.ModTime(),
			Compressed: compressed,
		})
	}
	return files, nil
}

func isManagedArchiveFile(path string) (bool, bool, error) {
	name := filepath.Base(path)
	compressed := strings.HasSuffix(name, ".gz")
	if !managedLogArchiveNamePattern.MatchString(name) && !legacyLogArchiveNamePattern.MatchString(name) {
		return false, compressed, nil
	}
	ok, err := hasManagedArchiveHeader(path, compressed)
	if err != nil {
		return false, compressed, fmt.Errorf("识别受管归档失败 %s: %w", name, err)
	}
	return ok, compressed, nil
}

func hasManagedArchiveHeader(path string, compressed bool) (bool, error) {
	handle, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer handle.Close()

	var reader io.Reader = handle
	if compressed {
		gzReader, err := gzip.NewReader(handle)
		if err != nil {
			return false, err
		}
		defer gzReader.Close()
		reader = gzReader
	}

	line, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	return strings.TrimSpace(line) == managedLogArchiveHeader, nil
}

func gzipArchiveFile(path string, modTime time.Time) (string, error) {
	source, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer source.Close()

	info, err := source.Stat()
	if err != nil {
		return "", err
	}

	dir := filepath.Dir(path)
	target := path + ".gz"
	tmp, err := os.CreateTemp(dir, ".ccclaw-archive-*.gz")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		cleanup()
		return "", err
	}
	gzWriter := gzip.NewWriter(tmp)
	if _, err := io.Copy(gzWriter, source); err != nil {
		_ = gzWriter.Close()
		cleanup()
		return "", err
	}
	if err := gzWriter.Close(); err != nil {
		cleanup()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := os.Chtimes(tmpPath, modTime, modTime); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if _, err := os.Stat(target); err == nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("目标文件已存在: %s", filepath.Base(target))
	} else if !os.IsNotExist(err) {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := os.Rename(tmpPath, target); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return target, nil
}

func sameArchivePath(left, right string) bool {
	return filepath.Clean(strings.TrimSpace(left)) == filepath.Clean(strings.TrimSpace(right))
}

func managedLogUnits(scope string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", "all":
		return []string{
			"ccclaw-ingest.service",
			"ccclaw-run.service",
			"ccclaw-patrol.service",
			"ccclaw-journal.service",
			"ccclaw-archive.service",
			"ccclaw-sevolver.service",
		}, nil
	case "ingest":
		return []string{"ccclaw-ingest.service"}, nil
	case "run":
		return []string{"ccclaw-run.service"}, nil
	case "patrol":
		return []string{"ccclaw-patrol.service"}, nil
	case "journal":
		return []string{"ccclaw-journal.service"}, nil
	case "archive":
		return []string{"ccclaw-archive.service"}, nil
	case "sevolver":
		return []string{"ccclaw-sevolver.service"}, nil
	default:
		return nil, fmt.Errorf("未知日志范围: %s (允许值: all|ingest|run|patrol|journal|archive|sevolver)", scope)
	}
}
