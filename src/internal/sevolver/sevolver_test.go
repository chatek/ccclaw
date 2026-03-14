package sevolver

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	ghadapter "github.com/41490/ccclaw/internal/adapters/github"
)

func TestRunResolvesClosedDeepAnalysisIssueBeforeComputingBacklog(t *testing.T) {
	kbDir := t.TempDir()
	now := time.Date(2026, 3, 13, 0, 0, 0, 0, time.Local)
	journalDir := filepath.Join(kbDir, "journal")
	skillPath := filepath.Join(kbDir, "skills", "L1", "demo", "CLAUDE.md")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte(`---
name: demo-skill
description: test
keywords: [demo]
status: active
gap_signals:
  - gap-20260312-demo
gap_escalations:
  - fingerprint: sg-demo
    status: escalated
    issue_number: 88
    issue_url: https://github.com/41490/ccclaw/issues/88
    updated_at: "2026-03-12"
    gap_ids:
      - gap-20260312-demo
---
content
`), 0o644); err != nil {
		t.Fatal(err)
	}

	backlog := []GapSignal{{
		ID:                    "gap-20260312-demo",
		Date:                  time.Date(2026, 3, 12, 0, 0, 0, 0, time.Local),
		Keyword:               "卡住",
		Source:                "2026/03/2026.03.12.demo.md",
		Context:               "部署卡住，只能先回滚",
		RelatedSkills:         []string{"skills/L1/demo/CLAUDE.md"},
		EscalationStatus:      gapEscalationStatusEscalated,
		EscalationFingerprint: "sg-demo",
		EscalationIssueNumber: 88,
		EscalationIssueURL:    "https://github.com/41490/ccclaw/issues/88",
		EscalationUpdatedAt:   "2026-03-12",
	}}
	if _, err := SaveGapSignals(kbDir, backlog, now); err != nil {
		t.Fatalf("SaveGapSignals failed: %v", err)
	}

	result, err := Run(Config{
		KBDir:       kbDir,
		JournalDir:  journalDir,
		ReportDir:   kbDir,
		ControlRepo: "41490/ccclaw",
		IssueClient: &fakeDeepAnalysisClient{
			issuesByID: map[int]ghadapter.Issue{
				88: {
					Repo:   "41490/ccclaw",
					Number: 88,
					State:  "closed",
					Labels: []ghadapter.Label{{Name: "fixed"}},
				},
			},
		},
		Now: now,
	}, io.Discard)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no non-blocking errors, got %#v", result.Errors)
	}
	if len(result.ResolvedIssues) != 1 || result.ResolvedIssues[0] != 88 {
		t.Fatalf("expected resolved issue #88, got %#v", result.ResolvedIssues)
	}
	if len(result.ResolvedGapIDs) != 1 || result.ResolvedGapIDs[0] != "gap-20260312-demo" {
		t.Fatalf("expected resolved gap, got %#v", result.ResolvedGapIDs)
	}
	if len(result.ResolvedSkills) != 1 || result.ResolvedSkills[0] != "skills/L1/demo/CLAUDE.md" {
		t.Fatalf("expected resolved skill, got %#v", result.ResolvedSkills)
	}
	if result.ResolvedIssueReasons[88] != closeReasonFixed {
		t.Fatalf("expected fixed reason, got %#v", result.ResolvedIssueReasons)
	}
	if result.ResolvedReasonCounters[closeReasonFixed] != 1 {
		t.Fatalf("expected fixed reason counter, got %#v", result.ResolvedReasonCounters)
	}
	if result.DeepAnalysis == nil || result.DeepAnalysis.Triggered {
		t.Fatalf("expected resolved backlog to skip deep-analysis trigger, got %#v", result.DeepAnalysis)
	}
}
