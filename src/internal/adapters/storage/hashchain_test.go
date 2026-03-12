package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/41490/ccclaw/internal/core"
)

func TestGenesisHash(t *testing.T) {
	if got, want := GenesisHash(2026, 11), "GENESIS-W2026-11"; got != want {
		t.Fatalf("unexpected genesis hash: got=%q want=%q", got, want)
	}
}

func TestVerifyChain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events-2026-W11.jsonl")
	records := []EventRecord{
		{Seq: 1, TaskID: "task-1", EventType: core.EventCreated, Detail: "{}", CreatedAt: time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC)},
		{Seq: 2, TaskID: "task-1", EventType: core.EventStarted, Detail: "{}", CreatedAt: time.Date(2026, 3, 12, 10, 1, 0, 0, time.UTC)},
		{Seq: 3, TaskID: "task-1", EventType: core.EventDone, Detail: "{}", CreatedAt: time.Date(2026, 3, 12, 10, 2, 0, 0, time.UTC)},
	}
	prevHash := GenesisHash(2026, 11)
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for idx := range records {
		records[idx].PrevHash = prevHash
		records[idx].Hash, err = ComputeHash(
			records[idx].TaskID,
			string(records[idx].EventType),
			records[idx].CreatedAt.UTC().Format(timeLayout),
			records[idx].Detail,
			prevHash,
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := encoder.Encode(records[idx]); err != nil {
			t.Fatal(err)
		}
		prevHash = records[idx].Hash
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := VerifyChain(path); err != nil {
		t.Fatalf("expected valid chain: %v", err)
	}
	records[1].Detail = `{"tampered":true}`
	file, err = os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	encoder = json.NewEncoder(file)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := VerifyChain(path); err == nil {
		t.Fatal("expected tampered chain to fail")
	}
}
