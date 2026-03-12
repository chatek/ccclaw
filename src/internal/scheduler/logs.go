package scheduler

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/41490/ccclaw/internal/logging"
)

type LogsOptions struct {
	Scope       string
	Follow      bool
	Since       string
	Lines       int
	Level       string
	ArchivePath string
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
	if strings.TrimSpace(options.ArchivePath) != "" {
		archive, err := openLogArchive(options)
		if err != nil {
			return err
		}
		defer archive.Close()
		logStdout = io.MultiWriter(stdout, archive)
		logStderr = io.MultiWriter(stderr, archive)
	}
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	cmd.Stdout = logStdout
	cmd.Stderr = logStderr
	cmd.Stdin = strings.NewReader("")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("执行 journalctl 失败: %w", err)
	}
	return nil
}

func BuildLogArchivePath(baseDir, scope string, now time.Time) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		scope = "all"
	}
	filename := fmt.Sprintf("%s_%s.log", now.Format("060102_150405"), scope)
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
	scope := strings.TrimSpace(options.Scope)
	if scope == "" {
		scope = "all"
	}
	header.WriteString("# ccclaw scheduler logs archive\n")
	header.WriteString(fmt.Sprintf("# archived_at: %s\n", time.Now().Format(time.RFC3339)))
	header.WriteString(fmt.Sprintf("# scope: %s\n", scope))
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

func managedLogUnits(scope string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", "all":
		return []string{
			"ccclaw-ingest.service",
			"ccclaw-run.service",
			"ccclaw-patrol.service",
			"ccclaw-journal.service",
		}, nil
	case "ingest":
		return []string{"ccclaw-ingest.service"}, nil
	case "run":
		return []string{"ccclaw-run.service"}, nil
	case "patrol":
		return []string{"ccclaw-patrol.service"}, nil
	case "journal":
		return []string{"ccclaw-journal.service"}, nil
	default:
		return nil, fmt.Errorf("未知日志范围: %s (允许值: all|ingest|run|patrol|journal)", scope)
	}
}
