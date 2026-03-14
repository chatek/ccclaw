package sevolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ghadapter "github.com/41490/ccclaw/internal/adapters/github"
)

func TestBackfillDeepAnalysisEscalationUpdatesGapLedgerAndSkillFrontmatter(t *testing.T) {
	kbDir := t.TempDir()
	now := time.Date(2026, 3, 12, 0, 0, 0, 0, time.Local)
	skillPath := filepath.Join(kbDir, "skills", "L1", "demo", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte(`---
name: demo-skill
description: test
keywords: [demo]
status: active
gap_signals: []
---
content
`), 0o644); err != nil {
		t.Fatal(err)
	}

	backlog := []GapSignal{{
		ID:      "gap-20260312-demo",
		Date:    now,
		Keyword: "卡住",
		Source:  "2026/03/2026.03.12.demo.md",
		Context: "部署卡住，只能先回滚",
	}}
	if _, err := SaveGapSignals(kbDir, backlog, now); err != nil {
		t.Fatalf("SaveGapSignals failed: %v", err)
	}

	backfill, err := BackfillDeepAnalysisEscalation(kbDir, backlog, &DeepAnalysisDecision{
		Triggered:   true,
		Existing:    true,
		IssueNumber: 88,
		IssueURL:    "https://github.com/41490/ccclaw/issues/88",
		Fingerprint: "sg-demo",
		RelevantGaps: []GapSignal{
			{ID: "gap-20260312-demo"},
		},
	}, []SkillHit{{
		SkillPath: "skills/L1/demo/CLAUDE.md",
		Source:    "2026/03/2026.03.12.demo.md",
		Date:      now,
	}}, now)
	if err != nil {
		t.Fatalf("BackfillDeepAnalysisEscalation failed: %v", err)
	}
	if len(backfill.GapIDs) != 1 || backfill.GapIDs[0] != "gap-20260312-demo" {
		t.Fatalf("unexpected backfilled gaps: %#v", backfill)
	}
	if len(backfill.SkillPaths) != 1 || backfill.SkillPaths[0] != "skills/L1/demo/CLAUDE.md" {
		t.Fatalf("unexpected backfilled skills: %#v", backfill)
	}

	loaded, err := LoadGapSignals(kbDir, now)
	if err != nil {
		t.Fatalf("LoadGapSignals failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected one gap after backfill, got %#v", loaded)
	}
	if loaded[0].EscalationStatus != gapEscalationStatusEscalated {
		t.Fatalf("expected escalated status, got %#v", loaded[0])
	}
	if loaded[0].EscalationFingerprint != "sg-demo" || loaded[0].EscalationIssueNumber != 88 {
		t.Fatalf("expected issue metadata on gap, got %#v", loaded[0])
	}
	if len(loaded[0].RelatedSkills) != 1 || loaded[0].RelatedSkills[0] != "skills/L1/demo/CLAUDE.md" {
		t.Fatalf("expected inferred related skill on gap, got %#v", loaded[0].RelatedSkills)
	}

	payload, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	for _, want := range []string{
		"gap_signals:",
		"- gap-20260312-demo",
		"gap_escalations:",
		"fingerprint: sg-demo",
		"status: escalated",
		"issue_number: 88",
		"issue_url: https://github.com/41490/ccclaw/issues/88",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in %q", want, text)
		}
	}
}

func TestResolveClosedDeepAnalysisEscalationsUpdatesGapLedgerAndSkillFrontmatter(t *testing.T) {
	kbDir := t.TempDir()
	now := time.Date(2026, 3, 13, 0, 0, 0, 0, time.Local)
	skillPath := filepath.Join(kbDir, "skills", "L1", "demo", "CLAUDE.md")
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

	updated, resolution, err := ResolveClosedDeepAnalysisEscalations(Config{
		KBDir:       kbDir,
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
	}, backlog, now)
	if err != nil {
		t.Fatalf("ResolveClosedDeepAnalysisEscalations failed: %v", err)
	}
	if len(resolution.IssueNumbers) != 1 || resolution.IssueNumbers[0] != 88 {
		t.Fatalf("unexpected resolved issues: %#v", resolution)
	}
	if len(resolution.GapIDs) != 1 || resolution.GapIDs[0] != "gap-20260312-demo" {
		t.Fatalf("unexpected resolved gaps: %#v", resolution)
	}
	if len(resolution.SkillPaths) != 1 || resolution.SkillPaths[0] != "skills/L1/demo/CLAUDE.md" {
		t.Fatalf("unexpected resolved skills: %#v", resolution)
	}
	if resolution.IssueCloseReasons[88] != closeReasonFixed {
		t.Fatalf("unexpected issue close reason: %#v", resolution.IssueCloseReasons)
	}
	if resolution.CloseReasonCounters[closeReasonFixed] != 1 {
		t.Fatalf("unexpected close reason counters: %#v", resolution.CloseReasonCounters)
	}
	if len(updated) != 1 || updated[0].EscalationStatus != gapEscalationStatusResolved {
		t.Fatalf("expected resolved backlog entry, got %#v", updated)
	}
	if updated[0].EscalationCloseReason != closeReasonFixed {
		t.Fatalf("expected fixed close reason on backlog, got %#v", updated[0])
	}

	loaded, err := LoadGapSignals(kbDir, now)
	if err != nil {
		t.Fatalf("LoadGapSignals failed: %v", err)
	}
	if len(loaded) != 1 || loaded[0].EscalationStatus != gapEscalationStatusResolved {
		t.Fatalf("expected resolved gap in ledger, got %#v", loaded)
	}
	if loaded[0].EscalationCloseReason != closeReasonFixed {
		t.Fatalf("expected fixed close reason in ledger, got %#v", loaded[0])
	}

	payload, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	for _, want := range []string{
		"fingerprint: sg-demo",
		"status: converged",
		"close_reason: fixed",
		"issue_number: 88",
		"updated_at: \"2026-03-13\"",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in %q", want, text)
		}
	}
}

func TestGapSignalRenderAndParseCloseReason(t *testing.T) {
	kbDir := t.TempDir()
	now := time.Date(2026, 3, 13, 0, 0, 0, 0, time.Local)
	gaps := []GapSignal{{
		ID:                    "gap-20260313-dup",
		Date:                  now,
		Keyword:               "失败",
		Source:                "2026/03/2026.03.13.md",
		Context:               "重复出现",
		EscalationStatus:      gapEscalationStatusResolved,
		EscalationFingerprint: "sg-dup",
		EscalationIssueNumber: 99,
		EscalationCloseReason: closeReasonDuplicate,
		EscalationUpdatedAt:   "2026-03-13",
	}}

	if _, err := SaveGapSignals(kbDir, gaps, now); err != nil {
		t.Fatalf("SaveGapSignals failed: %v", err)
	}
	loaded, err := LoadGapSignals(kbDir, now)
	if err != nil {
		t.Fatalf("LoadGapSignals failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 gap, got %d", len(loaded))
	}
	if loaded[0].EscalationCloseReason != closeReasonDuplicate {
		t.Fatalf("EscalationCloseReason: got %q, want %q", loaded[0].EscalationCloseReason, closeReasonDuplicate)
	}
}

func TestInferIssueCloseReasonPriorityBodyLabelComment(t *testing.T) {
	issue := &ghadapter.Issue{
		Repo:   "41490/ccclaw",
		Number: 101,
		State:  "closed",
		Body: `task_class: sevolver_deep_analysis
state_reason: cannot_reproduce
`,
		Labels: []ghadapter.Label{{Name: "duplicate"}},
	}
	comments := []ghadapter.Comment{
		{ID: 1, Body: "close_reason: fixed"},
		{ID: 2, Body: "任务完成\n\n/ccclaw [DONE]"},
	}
	got := inferIssueCloseReason(issue, comments)
	if got != closeReasonCannotReproduce {
		t.Fatalf("inferIssueCloseReason mismatch: got=%q want=%q", got, closeReasonCannotReproduce)
	}
}

func TestInferIssueCloseReasonFromDoneComment(t *testing.T) {
	issue := &ghadapter.Issue{
		Repo:   "41490/ccclaw",
		Number: 102,
		State:  "closed",
	}
	comments := []ghadapter.Comment{
		{ID: 11, Body: "过程记录"},
		{ID: 12, Body: "close_reason: duplicate\n\n/ccclaw [DONE]"},
	}
	got := inferIssueCloseReason(issue, comments)
	if got != closeReasonDuplicate {
		t.Fatalf("inferIssueCloseReason mismatch: got=%q want=%q", got, closeReasonDuplicate)
	}
}

func TestFilterActiveGapSignalsExcludesResolvedEntries(t *testing.T) {
	now := time.Date(2026, 3, 13, 0, 0, 0, 0, time.Local)
	active := filterActiveGapSignals([]GapSignal{
		{
			ID:               "gap-resolved",
			Date:             now.AddDate(0, 0, -1),
			Keyword:          "失败",
			EscalationStatus: gapEscalationStatusResolved,
		},
		{
			ID:               "gap-escalated",
			Date:             now,
			Keyword:          "失败",
			EscalationStatus: gapEscalationStatusEscalated,
		},
		{
			ID:      "gap-open",
			Date:    now,
			Keyword: "卡住",
		},
	})
	if len(active) != 2 {
		t.Fatalf("expected 2 active gaps, got %#v", active)
	}
	for _, item := range active {
		if item.ID == "gap-resolved" {
			t.Fatalf("resolved gap should be excluded: %#v", active)
		}
	}
}
