package sevolver

import (
	"fmt"
	"strings"
	"time"

	"github.com/41490/ccclaw/internal/adapters/storage"
	"github.com/41490/ccclaw/internal/core"
)

func ScanTaskEventsForGaps(stateDBPath, kbDir string, since time.Time) ([]GapSignal, error) {
	path := strings.TrimSpace(stateDBPath)
	if path == "" {
		return nil, nil
	}
	store, err := storage.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开状态存储失败: %w", err)
	}
	defer store.Close()
	return scanTaskEventsForGapsFromStore(store, kbDir, since)
}

func scanTaskEventsForGapsFromStore(store *storage.Store, kbDir string, since time.Time) ([]GapSignal, error) {
	if store == nil {
		return nil, nil
	}
	records, err := store.ListAllTaskEventsBetween(since, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("读取 task_events 失败: %w", err)
	}
	keywords, err := loadGapKeywords(kbDir)
	if err != nil {
		return nil, err
	}

	gaps := make([]GapSignal, 0, len(records))
	for _, record := range records {
		if !isTaskEventGapCandidate(record.EventType) {
			continue
		}
		keyword := matchTaskEventKeyword(record, keywords)
		if keyword == "" {
			continue
		}
		day := record.CreatedAt
		if day.IsZero() {
			continue
		}
		context := strings.TrimSpace(record.Detail)
		if context == "" {
			context = string(record.EventType)
		} else {
			context = fmt.Sprintf("%s | %s", record.EventType, context)
		}
		source := fmt.Sprintf("task_events/%s/%s#%d", strings.ToLower(string(record.EventType)), record.TaskID, record.Seq)
		gaps = append(gaps, GapSignal{
			ID:      buildGapID(day, source, 0, keyword, context),
			Date:    day,
			Keyword: keyword,
			Context: context,
			Source:  source,
		})
	}
	sortGapSignals(gaps)
	return gaps, nil
}

func isTaskEventGapCandidate(eventType core.EventType) bool {
	switch eventType {
	case core.EventBlocked, core.EventFailed, core.EventDead, core.EventWarning:
		return true
	default:
		return false
	}
}

func matchTaskEventKeyword(record storage.EventRecord, keywords []string) string {
	detail := strings.ToLower(strings.TrimSpace(record.Detail))
	for _, keyword := range keywords {
		if strings.Contains(detail, strings.ToLower(keyword)) {
			return keyword
		}
	}
	switch record.EventType {
	case core.EventFailed, core.EventDead:
		return "失败"
	case core.EventBlocked:
		return "阻塞"
	default:
		return ""
	}
}
