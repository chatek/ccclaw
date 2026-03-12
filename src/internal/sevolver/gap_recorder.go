package sevolver

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	gapRetentionDays   = 30
	managedStartMarker = "<!-- ccclaw:managed:start -->"
	managedEndMarker   = "<!-- ccclaw:managed:end -->"
	userStartMarker    = "<!-- ccclaw:user:start -->"
	userEndMarker      = "<!-- ccclaw:user:end -->"
	defaultUserStub    = "<!-- 本区块留给本机人工补充；自动更新时会保留这里的内容。 -->"
	gapSignalsTitle    = "# gap-signals"
	reportDefaultTitle = "# ccclaw sevolver"
)

func AppendGapSignals(kbDir string, gaps []GapSignal, now time.Time) (string, error) {
	path := filepath.Join(strings.TrimSpace(kbDir), "assay", "gap-signals.md")
	existing, userBody, err := readManagedMarkdownFile(path, gapSignalsTitle)
	if err != nil {
		return "", err
	}
	items := append(parseGapSignalEntries(existing), gaps...)
	pruned := pruneGapEntries(items, now)
	body := renderGapSignalBody(pruned)
	if err := writeManagedMarkdownFile(path, gapSignalsTitle, body, userBody); err != nil {
		return "", err
	}
	return path, nil
}

func parseGapSignalEntries(body string) []GapSignal {
	lines := strings.Split(body, "\n")
	items := make([]GapSignal, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		line = strings.TrimPrefix(line, "- [")
		idx := strings.Index(line, "] ")
		if idx < 0 {
			continue
		}
		id := strings.TrimSpace(line[:idx])
		parts := strings.Split(line[idx+2:], " | ")
		if len(parts) < 4 {
			continue
		}
		day, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(parts[0]), time.Local)
		if err != nil {
			continue
		}
		items = append(items, GapSignal{
			ID:      id,
			Date:    day,
			Keyword: strings.TrimSpace(parts[1]),
			Source:  strings.TrimSpace(parts[2]),
			Context: strings.TrimSpace(parts[3]),
		})
	}
	return items
}

func pruneGapEntries(items []GapSignal, now time.Time) []GapSignal {
	cutoff := dateFloor(now).AddDate(0, 0, -gapRetentionDays)
	seen := map[string]struct{}{}
	kept := make([]GapSignal, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			item.ID = buildGapID(item.Date, item.Source, 0, item.Keyword, item.Context)
		}
		if !item.Date.IsZero() && item.Date.Before(cutoff) {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		kept = append(kept, item)
	}
	sort.Slice(kept, func(i, j int) bool {
		if !kept[i].Date.Equal(kept[j].Date) {
			return kept[i].Date.Before(kept[j].Date)
		}
		return kept[i].ID < kept[j].ID
	})
	return kept
}

func renderGapSignalBody(items []GapSignal) string {
	if len(items) == 0 {
		return "暂无未清理的缺口信号。"
	}
	lines := make([]string, 0, len(items)+1)
	lines = append(lines, "以下条目由 `ccclaw sevolver` 自动维护，保留最近 30 天。", "")
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- [%s] %s | %s | %s | %s",
			item.ID,
			item.Date.Format("2006-01-02"),
			item.Keyword,
			item.Source,
			item.Context,
		))
	}
	return strings.Join(lines, "\n")
}

func readManagedMarkdownFile(path, defaultTitle string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", defaultUserStub, nil
		}
		return "", "", fmt.Errorf("读取受管 Markdown 失败: %w", err)
	}
	text := string(data)
	managedBody := betweenMarkers(text, managedStartMarker, managedEndMarker)
	userBody := betweenMarkers(text, userStartMarker, userEndMarker)
	if strings.TrimSpace(userBody) == "" {
		userBody = defaultUserStub
	}
	if strings.TrimSpace(managedBody) == "" {
		managedBody = strings.TrimSpace(strings.TrimPrefix(text, defaultTitle))
	}
	return strings.TrimSpace(managedBody), strings.TrimSpace(userBody), nil
}

func writeManagedMarkdownFile(path, title, managedBody, userBody string) error {
	if strings.TrimSpace(userBody) == "" {
		userBody = defaultUserStub
	}
	var builder strings.Builder
	builder.WriteString(title)
	builder.WriteString("\n\n")
	builder.WriteString(managedStartMarker)
	builder.WriteString("\n")
	builder.WriteString(strings.TrimSpace(managedBody))
	builder.WriteString("\n")
	builder.WriteString(managedEndMarker)
	builder.WriteString("\n\n")
	builder.WriteString(userStartMarker)
	builder.WriteString("\n")
	builder.WriteString(strings.TrimSpace(userBody))
	builder.WriteString("\n")
	builder.WriteString(userEndMarker)
	builder.WriteString("\n")
	return writeAtomically(path, []byte(builder.String()), 0o644)
}

func betweenMarkers(text, start, end string) string {
	startIdx := strings.Index(text, start)
	endIdx := strings.Index(text, end)
	if startIdx < 0 || endIdx < 0 || endIdx <= startIdx {
		return ""
	}
	return strings.TrimSpace(text[startIdx+len(start) : endIdx])
}
