package sevolver

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/41490/ccclaw/internal/adapters/storage"
	"github.com/41490/ccclaw/internal/core"
)

func TestScanTaskEventsForGapsIncludesFailureAndBlockedSignals(t *testing.T) {
	root := t.TempDir()
	store, err := storage.Open(filepath.Join(root, "var"))
	if err != nil {
		t.Fatalf("打开 store 失败: %v", err)
	}
	defer store.Close()

	base := time.Date(2026, 3, 13, 9, 0, 0, 0, time.Local)
	if err := store.AppendEventAt("task-1", core.EventFailed, "执行器退出码=1", base); err != nil {
		t.Fatalf("写入 failed 事件失败: %v", err)
	}
	if err := store.AppendEventAt("task-2", core.EventBlocked, "Issue 当前状态为 `blocked`", base.Add(2*time.Minute)); err != nil {
		t.Fatalf("写入 blocked 事件失败: %v", err)
	}
	if err := store.AppendEventAt("task-3", core.EventDone, "任务执行完成", base.Add(4*time.Minute)); err != nil {
		t.Fatalf("写入 done 事件失败: %v", err)
	}

	gaps, err := scanTaskEventsForGapsFromStore(store, root, base.Add(-time.Hour))
	if err != nil {
		t.Fatalf("scanTaskEventsForGapsFromStore failed: %v", err)
	}
	if len(gaps) != 2 {
		t.Fatalf("expected 2 gaps, got %#v", gaps)
	}
	if gaps[0].Keyword != "失败" {
		t.Fatalf("expected failure fallback keyword, got %#v", gaps[0])
	}
	if gaps[1].Keyword != "阻塞" {
		t.Fatalf("expected blocked fallback keyword, got %#v", gaps[1])
	}
	if gaps[0].Source == "" || gaps[1].Source == "" {
		t.Fatalf("expected task event source metadata, got %#v", gaps)
	}
}

func TestScanTaskEventsForGapsPrefersStructuredMetadata(t *testing.T) {
	root := t.TempDir()
	store, err := storage.Open(filepath.Join(root, "var"))
	if err != nil {
		t.Fatalf("打开 store 失败: %v", err)
	}
	defer store.Close()

	base := time.Date(2026, 3, 13, 10, 0, 0, 0, time.Local)
	if err := store.AppendEventAtStructured(
		"task-structured",
		core.EventWarning,
		"network jitter",
		base,
		storage.EventGapMeta{
			GapKeyword:      "重试",
			GapScope:        "finalize",
			GapSource:       "task_events/finalize/task-structured#1",
			RootCauseHint:   "git push timeout",
			GapAggregatable: true,
			GapEscalatable:  true,
		},
	); err != nil {
		t.Fatalf("写入结构化 warning 事件失败: %v", err)
	}
	if err := store.AppendEventAtStructured(
		"task-ignored",
		core.EventWarning,
		"duplicate warning",
		base.Add(2*time.Minute),
		storage.EventGapMeta{
			GapKeyword:      "失败",
			GapAggregatable: false,
		},
	); err != nil {
		t.Fatalf("写入不可聚合 warning 事件失败: %v", err)
	}

	gaps, err := scanTaskEventsForGapsFromStore(store, root, base.Add(-time.Hour))
	if err != nil {
		t.Fatalf("scanTaskEventsForGapsFromStore failed: %v", err)
	}
	if len(gaps) != 1 {
		t.Fatalf("expected 1 structured gap, got %#v", gaps)
	}
	if gaps[0].Keyword != "重试" {
		t.Fatalf("expected structured keyword, got %#v", gaps[0])
	}
	if gaps[0].Source != "task_events/finalize/task-structured#1" {
		t.Fatalf("expected structured source, got %#v", gaps[0])
	}
	if gaps[0].Context == "" || gaps[0].Context == "network jitter" {
		t.Fatalf("expected structured context with root cause, got %#v", gaps[0])
	}
}
