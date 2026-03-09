package storage

import (
	"path/filepath"
	"testing"

	"github.com/41490/ccclaw/internal/core"
)

func TestUpsertAndList(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	task := &core.Task{
		TaskID:         core.TaskID(3),
		IdempotencyKey: core.IdempotencyKey(3),
		ControlRepo:    "41490/ccclaw",
		TargetRepo:     "41490/ccclaw",
		IssueNumber:    3,
		IssueTitle:     "phase0",
		IssueAuthor:    "ZoomQuiet",
		Labels:         []string{"ccclaw"},
		Intent:         core.IntentResearch,
		RiskLevel:      core.RiskLow,
		State:          core.StateNew,
	}
	if err := store.UpsertTask(task); err != nil {
		t.Fatal(err)
	}
	tasks, err := store.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
}
