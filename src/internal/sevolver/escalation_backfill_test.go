package sevolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
