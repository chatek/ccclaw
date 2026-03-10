package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/41490/ccclaw/internal/core"
)

func TestStorePersistsLastSessionIDAndTokenStats(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("打开 store 失败: %v", err)
	}
	defer store.Close()

	task := &core.Task{
		TaskID:         "10#body",
		IdempotencyKey: "10#body",
		ControlRepo:    "41490/ccclaw",
		TargetRepo:     "41490/ccclaw",
		LastSessionID:  "sess-10",
		IssueNumber:    10,
		IssueTitle:     "phase1 stats",
		Labels:         []string{"ccclaw"},
		Intent:         core.IntentResearch,
		RiskLevel:      core.RiskLow,
		State:          core.StateDone,
	}
	if err := store.UpsertTask(task); err != nil {
		t.Fatalf("写入任务失败: %v", err)
	}
	if err := store.RecordTokenUsage(TokenUsageRecord{
		TaskID:    task.TaskID,
		SessionID: "sess-10",
		Usage: core.TokenUsage{
			InputTokens:              100,
			OutputTokens:             40,
			CacheCreationInputTokens: 10,
			CacheReadInputTokens:     5,
		},
		CostUSD:    0.42,
		DurationMS: 9000,
		RecordedAt: time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("写入 token 记录失败: %v", err)
	}

	loaded, err := store.GetByIdempotency(task.IdempotencyKey)
	if err != nil {
		t.Fatalf("读取任务失败: %v", err)
	}
	if loaded == nil || loaded.LastSessionID != "sess-10" {
		t.Fatalf("unexpected task session: %#v", loaded)
	}

	summary, err := store.TokenStats()
	if err != nil {
		t.Fatalf("读取统计失败: %v", err)
	}
	if summary.Runs != 1 || summary.Sessions != 1 {
		t.Fatalf("unexpected summary counts: %#v", summary)
	}
	if summary.InputTokens != 100 || summary.OutputTokens != 40 {
		t.Fatalf("unexpected summary tokens: %#v", summary)
	}
	if summary.CostUSD != 0.42 {
		t.Fatalf("unexpected summary cost: %#v", summary)
	}

	stats, err := store.TaskTokenStats(10)
	if err != nil {
		t.Fatalf("读取任务维度统计失败: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("unexpected stats count: %d", len(stats))
	}
	if stats[0].IssueNumber != 10 || stats[0].LastSessionID != "sess-10" {
		t.Fatalf("unexpected task stat: %#v", stats[0])
	}
}
