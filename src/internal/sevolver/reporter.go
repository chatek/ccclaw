package sevolver

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func WriteDailyReport(kbDir string, now time.Time, result Result) (string, error) {
	reportDir := filepath.Join(strings.TrimSpace(kbDir), "journal", "sevolver", now.Format("2006"), now.Format("01"))
	path := filepath.Join(reportDir, fmt.Sprintf("%s.log.md", now.Format("2006.01.02")))

	lines := []string{
		fmt.Sprintf("- 扫描窗口: %s -> %s", result.WindowStart.Format("2006-01-02"), result.Now.Format("2006-01-02")),
		fmt.Sprintf("- Skill 命中: %d", len(result.Hits)),
		fmt.Sprintf("- 缺口信号: %d", len(result.Gaps)),
		fmt.Sprintf("- dormant 处理: %d", len(result.Dormant)),
		fmt.Sprintf("- deprecated 归档: %d", len(result.Deprecated)),
	}
	if len(result.Errors) > 0 {
		lines = append(lines, fmt.Sprintf("- 非阻断告警: %d", len(result.Errors)))
	}

	if len(result.Hits) > 0 {
		lines = append(lines, "", "## Skill 命中")
		for _, hit := range result.Hits {
			lines = append(lines, fmt.Sprintf("- %s | %s | %s", hit.Date.Format("2006-01-02"), hit.SkillPath, hit.Source))
		}
	}
	if len(result.Gaps) > 0 {
		lines = append(lines, "", "## 缺口信号")
		for _, gap := range result.Gaps {
			lines = append(lines, fmt.Sprintf("- [%s] %s | %s | %s | %s", gap.ID, gap.Date.Format("2006-01-02"), gap.Keyword, gap.Source, gap.Context))
		}
	}
	if len(result.Dormant) > 0 {
		lines = append(lines, "", "## dormant")
		for _, item := range result.Dormant {
			lines = append(lines, "- "+filepath.ToSlash(item))
		}
	}
	if len(result.Deprecated) > 0 {
		lines = append(lines, "", "## deprecated")
		for _, item := range result.Deprecated {
			lines = append(lines, "- "+filepath.ToSlash(item))
		}
	}
	if len(result.Errors) > 0 {
		lines = append(lines, "", "## 告警")
		for _, item := range result.Errors {
			lines = append(lines, "- "+item)
		}
	}

	body := strings.Join(lines, "\n")
	title := fmt.Sprintf("%s %s", reportDefaultTitle, now.Format("2006-01-02"))
	if err := writeManagedMarkdownFile(path, title, body, defaultUserStub); err != nil {
		return "", err
	}
	return path, nil
}
