package core

import (
	"testing"
	"time"
)

func TestSafeReportFileName(t *testing.T) {
	got := SafeReportFileName(time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC), 3, "Phase0 Bootstrap / MVP")
	want := "260308_3_phase0_bootstrap_mvp.md"
	if got != want {
		t.Fatalf("unexpected filename: got %q want %q", got, want)
	}
}

func TestInferRisk(t *testing.T) {
	if InferRisk([]string{"ccclaw", "risk:high"}) != RiskHigh {
		t.Fatal("expected high risk")
	}
}
