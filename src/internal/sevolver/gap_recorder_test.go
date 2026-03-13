package sevolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendGapSignalsPrunesAndPreservesUserBlock(t *testing.T) {
	kbDir := t.TempDir()
	path := filepath.Join(kbDir, "assay", "gap-signals.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `# gap-signals

<!-- ccclaw:managed:start -->
- [gap-20260101-dead] 2026-01-01 | 失败 | journal/2026/01/demo.md | old
<!-- ccclaw:managed:end -->

<!-- ccclaw:user:start -->
人工备注
<!-- ccclaw:user:end -->
`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	updatedPath, err := AppendGapSignals(kbDir, []GapSignal{{
		ID:            "gap-20260312-new",
		Date:          time.Date(2026, 3, 12, 0, 0, 0, 0, time.Local),
		Keyword:       "失败",
		Source:        "journal/2026/03/demo.md",
		Context:       "部署失败",
		RelatedSkills: []string{"skills/L2/deploy-flow/CLAUDE.md"},
	}}, time.Date(2026, 3, 12, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("AppendGapSignals failed: %v", err)
	}
	if updatedPath != path {
		t.Fatalf("unexpected gap path: %s", updatedPath)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	if strings.Contains(text, "gap-20260101-dead") {
		t.Fatalf("old gap should be pruned: %q", text)
	}
	if !strings.Contains(text, "gap-20260312-new") {
		t.Fatalf("new gap should exist: %q", text)
	}
	if !strings.Contains(text, "related_skills: skills/L2/deploy-flow/CLAUDE.md") {
		t.Fatalf("related skills should be written: %q", text)
	}
	if !strings.Contains(text, "人工备注") {
		t.Fatalf("user block should be preserved: %q", text)
	}

	loaded, err := LoadGapSignals(kbDir, time.Date(2026, 3, 12, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("LoadGapSignals failed: %v", err)
	}
	if len(loaded) != 1 || loaded[0].ID != "gap-20260312-new" {
		t.Fatalf("unexpected loaded gaps: %#v", loaded)
	}
	if len(loaded[0].RelatedSkills) != 1 || loaded[0].RelatedSkills[0] != "skills/L2/deploy-flow/CLAUDE.md" {
		t.Fatalf("expected related skill metadata to round-trip, got %#v", loaded[0].RelatedSkills)
	}
}
