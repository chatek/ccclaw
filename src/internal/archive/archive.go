package archive

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/41490/ccclaw/internal/adapters/query"
	"github.com/41490/ccclaw/internal/adapters/storage"
)

type Result struct {
	Archived []string
	Skipped  []string
}

func Run(ctx context.Context, varDir string, out io.Writer) (*Result, error) {
	return RunAt(ctx, varDir, out, time.Now())
}

func RunAt(ctx context.Context, varDir string, out io.Writer, now time.Time) (*Result, error) {
	client := query.New()
	if strings.TrimSpace(client.Binary()) == "" {
		return nil, fmt.Errorf("未找到 duckdb CLI")
	}
	archiveDir := filepath.Join(varDir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建 archive 目录失败: %w", err)
	}

	currentYear, currentWeek := now.ISOWeek()

	patterns := []struct {
		glob   string
		verify func(string) error
	}{
		{glob: filepath.Join(varDir, "events-*.jsonl"), verify: storage.VerifyChain},
		{glob: filepath.Join(varDir, "token-*.jsonl"), verify: storage.VerifyTokenChain},
	}

	result := &Result{}
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern.glob)
		if err != nil {
			return nil, fmt.Errorf("枚举归档源失败: %w", err)
		}
		sort.Strings(files)
		for _, path := range files {
			year, week, ok := parseWeeklyFile(path)
			if !ok {
				result.Skipped = append(result.Skipped, fmt.Sprintf("%s: 文件名不匹配周分区约定", filepath.Base(path)))
				continue
			}
			if year == currentYear && week == currentWeek {
				result.Skipped = append(result.Skipped, fmt.Sprintf("%s: 当前周活动文件，跳过导出", filepath.Base(path)))
				continue
			}
			target := filepath.Join(archiveDir, strings.TrimSuffix(filepath.Base(path), ".jsonl")+".parquet")
			if skip, reason, err := shouldSkipArchive(path, target); err != nil {
				return nil, err
			} else if skip {
				result.Skipped = append(result.Skipped, fmt.Sprintf("%s: %s", filepath.Base(path), reason))
				continue
			}
			if err := pattern.verify(path); err != nil {
				return nil, fmt.Errorf("归档前校验失败: %s: %w", filepath.Base(path), err)
			}
			if err := client.CopyJSONLToParquet(ctx, path, target); err != nil {
				return nil, fmt.Errorf("导出 parquet 失败: %s: %w", filepath.Base(path), err)
			}
			result.Archived = append(result.Archived, fmt.Sprintf("%s -> %s", filepath.Base(path), filepath.Base(target)))
			if out != nil {
				_, _ = fmt.Fprintf(out, "archived %s -> %s\n", filepath.Base(path), filepath.Base(target))
			}
		}
	}
	if len(result.Archived) == 0 && out != nil {
		_, _ = fmt.Fprintln(out, "archive complete: no stale weekly JSONL to export")
	} else if out != nil {
		_, _ = fmt.Fprintln(out, "archive complete")
	}
	return result, nil
}

func shouldSkipArchive(sourcePath, targetPath string) (bool, string, error) {
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return false, "", fmt.Errorf("读取归档源失败: %w", err)
	}
	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("读取归档目标失败: %w", err)
	}
	if !targetInfo.ModTime().Before(sourceInfo.ModTime()) {
		return true, "parquet 已是最新快照", nil
	}
	return false, "", nil
}

func parseWeeklyFile(path string) (int, int, bool) {
	base := filepath.Base(path)
	parts := strings.Split(strings.TrimSuffix(base, ".jsonl"), "-")
	if len(parts) < 3 {
		return 0, 0, false
	}
	var year, week int
	if _, err := fmt.Sscanf(parts[len(parts)-2], "%d", &year); err != nil {
		return 0, 0, false
	}
	if _, err := fmt.Sscanf(strings.TrimPrefix(parts[len(parts)-1], "W"), "%d", &week); err != nil {
		return 0, 0, false
	}
	return year, week, true
}
