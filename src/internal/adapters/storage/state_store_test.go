package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/41490/ccclaw/internal/core"
)

func TestStateStorePersistsTaskSnapshot(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "var"))
	if err != nil {
		t.Fatal(err)
	}
	task := &core.Task{
		TaskID:         "42#body",
		IdempotencyKey: "42#body",
		IssueRepo:      "41490/ccclaw",
		TargetRepo:     "41490/ccclaw",
		IssueNumber:    42,
		IssueTitle:     "phase1 state",
		TaskClass:      core.TaskClassSevolverDeepAnalysis,
		State:          core.StateRunning,
		LastSessionID:  "sess-42",
		UpdatedAt:      time.Date(2026, 3, 12, 11, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertTask(task); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetByIdempotency(task.IdempotencyKey)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil {
		t.Fatal("expected persisted task")
	}
	if loaded.LastSessionID != "sess-42" || loaded.State != core.StateRunning || loaded.TaskClass != core.TaskClassSevolverDeepAnalysis {
		t.Fatalf("unexpected task snapshot: %#v", loaded)
	}
	tasks, err := store.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != task.TaskID {
		t.Fatalf("unexpected task list: %#v", tasks)
	}
}
