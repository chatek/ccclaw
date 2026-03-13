package app

import (
	"testing"

	"github.com/41490/ccclaw/internal/core"
	"github.com/41490/ccclaw/internal/executor"
)

func TestMapStreamSnapshotUsesContractMapping(t *testing.T) {
	snapshot := &executor.StreamEventSnapshot{
		CurrentStep: "execute",
		Mapping: executor.StreamStateMapping{
			RepoSlotPhase:   executor.StreamSlotPhaseFinalizing,
			RepoSlotStep:    "sync_target",
			TaskState:       core.StateFinalizing,
			TaskEvent:       core.EventUpdated,
			TaskEventDetail: "stream-json 结果已就绪，进入收口",
		},
	}
	phase, step, state, eventType, detail := mapStreamSnapshot(snapshot)
	if phase != "finalizing" || step != "sync_target" || state != core.StateFinalizing || eventType != core.EventUpdated {
		t.Fatalf("unexpected mapping: phase=%s step=%s state=%s event=%s", phase, step, state, eventType)
	}
	if detail == "" {
		t.Fatal("预期返回 task_event_detail")
	}
}

func TestMapStreamSnapshotFallsBackToSafeDefaults(t *testing.T) {
	snapshot := &executor.StreamEventSnapshot{
		CurrentStep: "load_result",
		Error:       "执行器退出码=1",
		Mapping: executor.StreamStateMapping{
			RepoSlotPhase: executor.StreamSlotPhase("unknown"),
			TaskState:     core.State("UNKNOWN"),
			TaskEvent:     core.EventType("UNKNOWN"),
		},
	}
	phase, step, state, eventType, detail := mapStreamSnapshot(snapshot)
	if phase != "running" || step != "load_result" {
		t.Fatalf("unexpected slot fallback: phase=%s step=%s", phase, step)
	}
	if state != core.StateRunning || eventType != core.EventUpdated {
		t.Fatalf("unexpected task fallback: state=%s event=%s", state, eventType)
	}
	if detail != snapshot.Error {
		t.Fatalf("unexpected detail fallback: %q", detail)
	}
}
