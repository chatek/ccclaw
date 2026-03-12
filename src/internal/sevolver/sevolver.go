package sevolver

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const scanWindowDays = 7

type Config struct {
	KBDir      string
	JournalDir string
	ReportDir  string
	Now        time.Time
}

type Result struct {
	Now         time.Time
	WindowStart time.Time
	Hits        []SkillHit
	Gaps        []GapSignal
	Dormant     []string
	Deprecated  []string
	ReportPath  string
	GapFilePath string
	Errors      []string
}

func Run(cfg Config, out io.Writer) (*Result, error) {
	now := cfg.Now
	if now.IsZero() {
		now = time.Now()
	}
	if strings.TrimSpace(cfg.KBDir) == "" {
		return nil, fmt.Errorf("KBDir 不能为空")
	}
	journalDir := strings.TrimSpace(cfg.JournalDir)
	if journalDir == "" {
		journalDir = filepath.Join(cfg.KBDir, "journal")
	}
	reportRoot := strings.TrimSpace(cfg.ReportDir)
	if reportRoot == "" {
		reportRoot = cfg.KBDir
	}

	result := &Result{
		Now:         dateFloor(now),
		WindowStart: dateFloor(now).AddDate(0, 0, -scanWindowDays),
	}
	if out != nil {
		_, _ = fmt.Fprintf(out, "sevolver: start %s\n", result.Now.Format("2006-01-02"))
	}

	hits, err := ScanJournal(journalDir, result.WindowStart)
	if err != nil {
		return nil, fmt.Errorf("扫描 journal skill 命中失败: %w", err)
	}
	result.Hits = hits

	gaps, err := ScanJournalForGaps(journalDir, cfg.KBDir, result.WindowStart)
	if err != nil {
		return nil, fmt.Errorf("扫描 journal 缺口信号失败: %w", err)
	}
	result.Gaps = gaps

	for _, hit := range hits {
		skillFile := filepath.Join(cfg.KBDir, filepath.FromSlash(hit.SkillPath))
		if _, err := os.Stat(skillFile); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("skill 不存在: %s", filepath.ToSlash(skillFile)))
			continue
		}
		if err := UpdateSkillMeta(skillFile, hit); err != nil {
			result.Errors = append(result.Errors, err.Error())
			continue
		}
	}

	lifecycleActions, err := processSkillLifecycle(cfg.KBDir, result.Now)
	if err != nil {
		return nil, err
	}
	for _, action := range lifecycleActions {
		switch action.Status {
		case skillStatusDormant:
			result.Dormant = append(result.Dormant, filepath.ToSlash(action.Path))
		case skillStatusDeprecated:
			result.Deprecated = append(result.Deprecated, filepath.ToSlash(action.Path))
		}
	}

	gapFilePath, err := AppendGapSignals(cfg.KBDir, gaps, result.Now)
	if err != nil {
		return nil, err
	}
	result.GapFilePath = gapFilePath

	reportPath, err := WriteDailyReport(reportRoot, result.Now, *result)
	if err != nil {
		return nil, err
	}
	result.ReportPath = reportPath

	if out != nil {
		_, _ = fmt.Fprintf(out, "sevolver: hits=%d gaps=%d dormant=%d deprecated=%d\n", len(result.Hits), len(result.Gaps), len(result.Dormant), len(result.Deprecated))
		if result.ReportPath != "" {
			_, _ = fmt.Fprintf(out, "sevolver: report=%s\n", result.ReportPath)
		}
		_, _ = fmt.Fprintln(out, "sevolver: done")
	}
	return result, nil
}
