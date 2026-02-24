package task_test

import (
	"os"
	"testing"

	"github.com/NOTAschool/ccclaw/internal/task"
)

func TestTaskCanExecute(t *testing.T) {
	tests := []struct {
		name    string
		tk      task.Task
		wantErr bool
	}{
		{
			name:    "low risk 可执行",
			tk:      task.Task{RiskLevel: task.RiskLow, RetryCount: 0},
			wantErr: false,
		},
		{
			name:    "high risk 未 approved 被阻止",
			tk:      task.Task{RiskLevel: task.RiskHigh, Approved: false, RetryCount: 0},
			wantErr: true,
		},
		{
			name:    "high risk 已 approved 可执行",
			tk:      task.Task{RiskLevel: task.RiskHigh, Approved: true, RetryCount: 0},
			wantErr: false,
		},
		{
			name:    "超过重试次数被阻止",
			tk:      task.Task{RiskLevel: task.RiskLow, RetryCount: 3},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.tk.CanExecute()
			if (err != nil) != tc.wantErr {
				t.Errorf("CanExecute() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestStoreIdempotency(t *testing.T) {
	dir := t.TempDir()
	store, err := task.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	tk := &task.Task{
		TaskID:         "123#body",
		IdempotencyKey: "123#body",
		IssueNumber:    123,
		State:          task.StateNew,
		RiskLevel:      task.RiskLow,
	}

	// 首次保存
	if err := store.Save(tk); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// 幂等键应存在
	exists, err := store.Exists("123#body")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Error("期望幂等键存在，但不存在")
	}

	// 不存在的键
	exists, err = store.Exists("999#body")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Error("期望幂等键不存在，但存在")
	}
}

func TestStoreLoadSave(t *testing.T) {
	dir := t.TempDir()
	store, err := task.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	tk := &task.Task{
		TaskID:         "42#body",
		IdempotencyKey: "42#body",
		IssueNumber:    42,
		IssueTitle:     "测试任务",
		State:          task.StateNew,
		RiskLevel:      task.RiskMed,
		RetryCount:     0,
	}

	if err := store.Save(tk); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load("42#body")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("Load 返回 nil")
	}
	if loaded.IssueTitle != "测试任务" {
		t.Errorf("IssueTitle = %q, want %q", loaded.IssueTitle, "测试任务")
	}
	if loaded.State != task.StateNew {
		t.Errorf("State = %q, want %q", loaded.State, task.StateNew)
	}
}

func TestStoreList(t *testing.T) {
	dir := t.TempDir()
	store, err := task.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	for i := 1; i <= 3; i++ {
		tk := &task.Task{
			TaskID:         string(rune('0'+i)) + "#body",
			IdempotencyKey: string(rune('0'+i)) + "#body",
			IssueNumber:    i,
			State:          task.StateNew,
			RiskLevel:      task.RiskLow,
		}
		if err := store.Save(tk); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	tasks, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("List 返回 %d 个任务，期望 3", len(tasks))
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
