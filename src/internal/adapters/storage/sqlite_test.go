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
		TaskID:     task.TaskID,
		SessionID:  "sess-10",
		PromptFile: "/tmp/prompts/10_body.md",
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

func TestStoreSupportsRTKComparisonAndJournalQueries(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("打开 store 失败: %v", err)
	}
	defer store.Close()

	day := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	tasks := []*core.Task{
		{
			TaskID:         "10#body",
			IdempotencyKey: "10#body",
			ControlRepo:    "41490/ccclaw",
			TargetRepo:     "41490/ccclaw",
			IssueNumber:    10,
			IssueTitle:     "rtk on",
			Labels:         []string{"ccclaw"},
			Intent:         core.IntentResearch,
			RiskLevel:      core.RiskLow,
			State:          core.StateDone,
		},
		{
			TaskID:         "11#body",
			IdempotencyKey: "11#body",
			ControlRepo:    "41490/ccclaw",
			TargetRepo:     "41490/ccclaw",
			IssueNumber:    11,
			IssueTitle:     "rtk off",
			Labels:         []string{"ccclaw"},
			Intent:         core.IntentResearch,
			RiskLevel:      core.RiskLow,
			State:          core.StateDead,
		},
	}
	for _, task := range tasks {
		if err := store.UpsertTask(task); err != nil {
			t.Fatalf("写入任务失败: %v", err)
		}
	}
	if err := store.AppendEvent("10#body", core.EventStarted, "开始执行"); err != nil {
		t.Fatalf("写入 started 事件失败: %v", err)
	}
	if err := store.AppendEvent("10#body", core.EventDone, "执行完成"); err != nil {
		t.Fatalf("写入 done 事件失败: %v", err)
	}
	if err := store.AppendEvent("11#body", core.EventDead, "tmux 会话超时"); err != nil {
		t.Fatalf("写入 dead 事件失败: %v", err)
	}
	if err := store.RecordTokenUsage(TokenUsageRecord{
		TaskID:     "10#body",
		SessionID:  "sess-10",
		PromptFile: "/tmp/prompts/10.md",
		Usage: core.TokenUsage{
			InputTokens:  60,
			OutputTokens: 20,
		},
		CostUSD:    0.10,
		DurationMS: 1000,
		RTKEnabled: true,
		RecordedAt: day,
	}); err != nil {
		t.Fatalf("写入 rtk token 失败: %v", err)
	}
	if err := store.RecordTokenUsage(TokenUsageRecord{
		TaskID:     "11#body",
		SessionID:  "sess-11",
		PromptFile: "/tmp/prompts/11.md",
		Usage: core.TokenUsage{
			InputTokens:  80,
			OutputTokens: 30,
		},
		CostUSD:    0.40,
		DurationMS: 1500,
		RTKEnabled: false,
		RecordedAt: day,
	}); err != nil {
		t.Fatalf("写入 plain token 失败: %v", err)
	}

	comparison, err := store.RTKComparisonBetween(day.Add(-time.Hour), day.Add(time.Hour))
	if err != nil {
		t.Fatalf("读取 rtk 对比失败: %v", err)
	}
	if comparison.RTKRuns != 1 || comparison.PlainRuns != 1 {
		t.Fatalf("unexpected comparison counts: %#v", comparison)
	}
	if comparison.SavingsPercent <= 0 {
		t.Fatalf("预期 rtk 节省为正数，实际为 %#v", comparison)
	}

	summary, err := store.JournalDaySummary(day)
	if err != nil {
		t.Fatalf("读取 journal 汇总失败: %v", err)
	}
	if summary.TasksTouched != 2 || summary.Done != 1 || summary.Dead != 1 {
		t.Fatalf("unexpected journal summary: %#v", summary)
	}

	items, err := store.JournalTaskSummaries(day)
	if err != nil {
		t.Fatalf("读取 journal 任务汇总失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("unexpected journal task count: %d", len(items))
	}

	events, err := store.ListTaskEventsBetween(day.Add(-24*time.Hour), day.Add(24*time.Hour), 10)
	if err != nil {
		t.Fatalf("读取事件失败: %v", err)
	}
	if len(events) < 3 {
		t.Fatalf("unexpected event count: %d", len(events))
	}
}
