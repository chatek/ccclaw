package storage

import (
	"strings"

	"github.com/41490/ccclaw/internal/core"
)

type EventGapMeta struct {
	GapKeyword      string
	GapScope        string
	GapSource       string
	RootCauseHint   string
	GapAggregatable bool
	GapEscalatable  bool
}

func inferEventGapMeta(eventType core.EventType, detail string) EventGapMeta {
	keyword := detectGapKeyword(detail)
	meta := EventGapMeta{
		GapScope:      "task_events",
		GapSource:     "task_events/" + strings.ToLower(string(eventType)),
		RootCauseHint: summarizeRootCause(detail),
	}
	switch eventType {
	case core.EventFailed, core.EventDead:
		if keyword == "" {
			keyword = "失败"
		}
		meta.GapAggregatable = true
		meta.GapEscalatable = true
	case core.EventBlocked:
		if keyword == "" {
			keyword = "阻塞"
		}
		meta.GapAggregatable = true
		meta.GapEscalatable = true
	case core.EventWarning:
		meta.GapAggregatable = keyword != ""
		meta.GapEscalatable = keyword != ""
	default:
		return EventGapMeta{}
	}
	meta.GapKeyword = keyword
	return meta.normalized()
}

func (meta EventGapMeta) normalized() EventGapMeta {
	meta.GapKeyword = strings.TrimSpace(meta.GapKeyword)
	meta.GapScope = strings.TrimSpace(meta.GapScope)
	meta.GapSource = strings.TrimSpace(meta.GapSource)
	meta.RootCauseHint = strings.TrimSpace(meta.RootCauseHint)
	return meta
}

func (meta EventGapMeta) merge(override EventGapMeta) EventGapMeta {
	base := meta.normalized()
	override = override.normalized()
	if override.GapKeyword != "" {
		base.GapKeyword = override.GapKeyword
	}
	if override.GapScope != "" {
		base.GapScope = override.GapScope
	}
	if override.GapSource != "" {
		base.GapSource = override.GapSource
	}
	if override.RootCauseHint != "" {
		base.RootCauseHint = override.RootCauseHint
	}
	if override.GapAggregatable {
		base.GapAggregatable = true
	}
	if override.GapEscalatable {
		base.GapEscalatable = true
	}
	return base
}

func detectGapKeyword(detail string) string {
	text := strings.ToLower(strings.TrimSpace(detail))
	switch {
	case text == "":
		return ""
	case strings.Contains(text, "无法复现") || strings.Contains(text, "cannot reproduce"):
		return "无法复现"
	case strings.Contains(text, "阻塞") || strings.Contains(text, "blocked"):
		return "阻塞"
	case strings.Contains(text, "超时") || strings.Contains(text, "timeout"):
		return "超时"
	case strings.Contains(text, "找不到") || strings.Contains(text, "not found"):
		return "找不到"
	case strings.Contains(text, "失败") || strings.Contains(text, "failed") || strings.Contains(text, "error") || strings.Contains(text, "panic"):
		return "失败"
	default:
		return ""
	}
}

func summarizeRootCause(detail string) string {
	text := strings.TrimSpace(strings.ReplaceAll(detail, "\r\n", "\n"))
	if text == "" {
		return ""
	}
	for _, sep := range []string{"\n", "；", ";", "|"} {
		if idx := strings.Index(text, sep); idx > 0 {
			text = strings.TrimSpace(text[:idx])
		}
	}
	if len(text) > 120 {
		return strings.TrimSpace(text[:117]) + "..."
	}
	return text
}
