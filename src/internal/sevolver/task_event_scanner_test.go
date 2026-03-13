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
	store, err := storage.Open(filepath.Join(root, "state.db"))
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
