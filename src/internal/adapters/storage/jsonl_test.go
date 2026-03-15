package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/41490/ccclaw/internal/core"
)

func TestJSONLStoreAppendsWeeklyRecords(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "var"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEventAt("task-1", core.EventCreated, "{}", time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEventAt("task-1", core.EventFailed, "执行器退出码=1", time.Date(2026, 3, 12, 10, 0, 30, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordTokenUsage(TokenUsageRecord{
		TaskID:     "task-1",
		SessionID:  "sess-1",
		Usage:      core.TokenUsage{InputTokens: 10, OutputTokens: 4},
		CostUSD:    0.12,
		RTKEnabled: true,
		RecordedAt: time.Date(2026, 3, 12, 10, 1, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	events, err := store.jsonl.ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("unexpected event count: %d", len(events))
	}
	if events[0].Seq != 1 || events[0].PrevHash != GenesisHash(2026, 11) || events[0].Hash == "" {
		t.Fatalf("unexpected event record: %#v", events[0])
	}
	if events[1].GapKeyword != "失败" || !events[1].GapAggregatable || !events[1].GapEscalatable {
		t.Fatalf("unexpected structured event metadata: %#v", events[1])
	}
	tokens, err := store.jsonl.ReadTokens()
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 1 {
		t.Fatalf("unexpected token count: %d", len(tokens))
	}
	if tokens[0].Seq != 1 || tokens[0].PrevHash != GenesisHash(2026, 11) || tokens[0].Hash == "" {
		t.Fatalf("unexpected token record: %#v", tokens[0])
	}
}
