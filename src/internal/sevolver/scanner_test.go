package sevolver

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanJournalFindsSkillHits(t *testing.T) {
	root := t.TempDir()
	journalDir := filepath.Join(root, "journal", "2026", "03")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(journalDir, "2026.03.12.demo.md")
	body := `# 2026-03-12

used kb/skills/L1/git-conflict/CLAUDE.md to resolve merge
also used kb/skills/L2/deploy-flow/CLAUDE.md for deployment
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	hits, err := ScanJournal(filepath.Join(root, "journal"), time.Date(2026, 3, 11, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("ScanJournal failed: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %#v", hits)
	}
	if hits[0].SkillPath != "skills/L1/git-conflict/CLAUDE.md" {
		t.Fatalf("unexpected first hit: %#v", hits[0])
	}
	if hits[1].SkillPath != "skills/L2/deploy-flow/CLAUDE.md" {
		t.Fatalf("unexpected second hit: %#v", hits[1])
	}
}

func TestScanJournalForGapsLoadsSignalRules(t *testing.T) {
	root := t.TempDir()
	journalDir := filepath.Join(root, "journal", "2026", "03")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "assay"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assay", "signal-rules.md"), []byte(`# signal-rules

- 关键词: 卡住, 回滚
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journalDir, "2026.03.12.demo.md"), []byte("部署卡住，只能先回滚\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	gaps, err := ScanJournalForGaps(filepath.Join(root, "journal"), root, time.Date(2026, 3, 11, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("ScanJournalForGaps failed: %v", err)
	}
	if len(gaps) != 1 {
		t.Fatalf("expected 1 gap, got %#v", gaps)
	}
	if gaps[0].Keyword != "卡住" {
		t.Fatalf("unexpected gap keyword: %#v", gaps[0])
	}
}
