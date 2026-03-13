package sevolver

import (
	"fmt"
	"os"
	"path/filepath"
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
	if err := writeGapSignals(path, userBody, items, now); err != nil {
		return "", err
	}
	return path, nil
}

func SaveGapSignals(kbDir string, gaps []GapSignal, now time.Time) (string, error) {
	path := filepath.Join(strings.TrimSpace(kbDir), "assay", "gap-signals.md")
	_, userBody, err := readManagedMarkdownFile(path, gapSignalsTitle)
	if err != nil {
		return "", err
	}
	if err := writeGapSignals(path, userBody, gaps, now); err != nil {
		return "", err
	}
	return path, nil
}

func LoadGapSignals(kbDir string, now time.Time) ([]GapSignal, error) {
	path := filepath.Join(strings.TrimSpace(kbDir), "assay", "gap-signals.md")
	existing, _, err := readManagedMarkdownFile(path, gapSignalsTitle)
	if err != nil {
		return nil, err
	}
	return pruneGapEntries(parseGapSignalEntries(existing), now), nil
}

func parseGapSignalEntries(body string) []GapSignal {
	lines := strings.Split(body, "\n")
	items := make([]GapSignal, 0)
	var current *GapSignal
	flush := func() {
		if current == nil {
			return
		}
		current.RelatedSkills = uniqueSortedStrings(current.RelatedSkills)
		items = append(items, *current)
		current = nil
	}
	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [") {
			flush()
			item, ok := parseGapSignalHeadline(trimmed)
			if ok {
				current = &item
			}
			continue
		}
		if current == nil {
			continue
		}
		if !strings.HasPrefix(line, "  ") {
			continue
		}
		parseGapSignalMetadata(current, strings.TrimSpace(line))
	}
	flush()
	return items
}

func pruneGapEntries(items []GapSignal, now time.Time) []GapSignal {
	cutoff := dateFloor(now).AddDate(0, 0, -gapRetentionDays)
	indexed := map[string]int{}
	kept := make([]GapSignal, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			item.ID = buildGapID(item.Date, item.Source, 0, item.Keyword, item.Context)
		}
		if !item.Date.IsZero() && item.Date.Before(cutoff) {
			continue
		}
		if idx, ok := indexed[item.ID]; ok {
			kept[idx] = mergeGapSignal(kept[idx], item)
			continue
		}
		item.RelatedSkills = uniqueSortedStrings(item.RelatedSkills)
		indexed[item.ID] = len(kept)
		kept = append(kept, item)
	}
	sortGapSignals(kept)
	return kept
}

func renderGapSignalBody(items []GapSignal) string {
	if len(items) == 0 {
		return "暂无未清理的缺口信号。"
	}
	lines := make([]string, 0, len(items)+1)
	lines = append(lines, "以下条目由 `ccclaw sevolver` 自动维护，保留最近 30 天。", "")
	for idx, item := range items {
		lines = append(lines, fmt.Sprintf("- [%s] %s | %s | %s | %s",
			item.ID,
			item.Date.Format("2006-01-02"),
			item.Keyword,
			item.Source,
			item.Context,
		))
		if len(item.RelatedSkills) > 0 {
			lines = append(lines, "  related_skills: "+strings.Join(uniqueSortedStrings(item.RelatedSkills), ", "))
		}
		if strings.TrimSpace(item.EscalationStatus) != "" {
			lines = append(lines, "  escalation_status: "+strings.TrimSpace(item.EscalationStatus))
		}
		if strings.TrimSpace(item.EscalationFingerprint) != "" {
			lines = append(lines, "  escalation_fingerprint: "+strings.TrimSpace(item.EscalationFingerprint))
		}
		if item.EscalationIssueNumber > 0 {
			lines = append(lines, fmt.Sprintf("  escalation_issue_number: %d", item.EscalationIssueNumber))
		}
		if strings.TrimSpace(item.EscalationIssueURL) != "" {
			lines = append(lines, "  escalation_issue_url: "+strings.TrimSpace(item.EscalationIssueURL))
		}
		if strings.TrimSpace(item.EscalationUpdatedAt) != "" {
			lines = append(lines, "  escalation_updated_at: "+strings.TrimSpace(item.EscalationUpdatedAt))
		}
		if idx < len(items)-1 {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}

func writeGapSignals(path, userBody string, items []GapSignal, now time.Time) error {
	pruned := pruneGapEntries(items, now)
	body := renderGapSignalBody(pruned)
	return writeManagedMarkdownFile(path, gapSignalsTitle, body, userBody)
}

func parseGapSignalHeadline(line string) (GapSignal, bool) {
	line = strings.TrimPrefix(line, "- [")
	idx := strings.Index(line, "] ")
	if idx < 0 {
		return GapSignal{}, false
	}
	id := strings.TrimSpace(line[:idx])
	parts := strings.SplitN(line[idx+2:], " | ", 4)
	if len(parts) < 4 {
		return GapSignal{}, false
	}
	day, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(parts[0]), time.Local)
	if err != nil {
		return GapSignal{}, false
	}
	return GapSignal{
		ID:      id,
		Date:    day,
		Keyword: strings.TrimSpace(parts[1]),
		Source:  strings.TrimSpace(parts[2]),
		Context: strings.TrimSpace(parts[3]),
	}, true
}

func parseGapSignalMetadata(item *GapSignal, line string) {
	if item == nil {
		return
	}
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return
	}
	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	switch key {
	case "related_skills":
		item.RelatedSkills = append(item.RelatedSkills, splitCommaSeparated(value)...)
	case "escalation_status":
		item.EscalationStatus = value
	case "escalation_fingerprint":
		item.EscalationFingerprint = value
	case "escalation_issue_number":
		fmt.Sscanf(value, "%d", &item.EscalationIssueNumber)
	case "escalation_issue_url":
		item.EscalationIssueURL = value
	case "escalation_updated_at":
		item.EscalationUpdatedAt = value
	}
}

func mergeGapSignal(current, incoming GapSignal) GapSignal {
	if current.Date.IsZero() && !incoming.Date.IsZero() {
		current.Date = incoming.Date
	}
	if strings.TrimSpace(current.Keyword) == "" {
		current.Keyword = incoming.Keyword
	}
	if strings.TrimSpace(current.Source) == "" {
		current.Source = incoming.Source
	}
	if strings.TrimSpace(current.Context) == "" {
		current.Context = incoming.Context
	}
	current.RelatedSkills = uniqueSortedStrings(append(current.RelatedSkills, incoming.RelatedSkills...))
	if strings.TrimSpace(current.EscalationStatus) == "" {
		current.EscalationStatus = incoming.EscalationStatus
	}
	if strings.TrimSpace(current.EscalationFingerprint) == "" {
		current.EscalationFingerprint = incoming.EscalationFingerprint
	}
	if current.EscalationIssueNumber == 0 {
		current.EscalationIssueNumber = incoming.EscalationIssueNumber
	}
	if strings.TrimSpace(current.EscalationIssueURL) == "" {
		current.EscalationIssueURL = incoming.EscalationIssueURL
	}
	if strings.TrimSpace(current.EscalationUpdatedAt) == "" {
		current.EscalationUpdatedAt = incoming.EscalationUpdatedAt
	}
	return current
}

func splitCommaSeparated(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	items := strings.Split(raw, ",")
	values := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		values = append(values, value)
	}
	return values
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
