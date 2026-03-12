package scheduler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectManagedUnitDriftReportsMissingAndDrifted(t *testing.T) {
	cfg := testSystemdConfig()
	cfg.Scheduler.SystemdUserDir = t.TempDir()

	units, err := GenerateSystemdUnitContents(cfg)
	if err != nil {
		t.Fatalf("生成 unit 失败: %v", err)
	}
	for name, content := range units {
		if err := os.WriteFile(filepath.Join(cfg.Scheduler.SystemdUserDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("写入 unit 失败: %v", err)
		}
	}

	if err := os.Remove(filepath.Join(cfg.Scheduler.SystemdUserDir, "ccclaw-run.timer")); err != nil {
		t.Fatalf("删除 unit 失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.Scheduler.SystemdUserDir, "ccclaw-patrol.service"), []byte("[Unit]\nDescription=drifted\n"), 0o644); err != nil {
		t.Fatalf("写入漂移 unit 失败: %v", err)
	}

	status, err := DetectManagedUnitDrift(cfg)
	if err != nil {
		t.Fatalf("检测 unit 漂移失败: %v", err)
	}
	if len(status.Missing) != 1 || status.Missing[0] != "ccclaw-run.timer" {
		t.Fatalf("unexpected missing units: %+v", status.Missing)
	}
	if len(status.Drifted) != 1 || status.Drifted[0] != "ccclaw-patrol.service" {
		t.Fatalf("unexpected drifted units: %+v", status.Drifted)
	}
}

func TestResolveTimersViewRejectsConflicts(t *testing.T) {
	if _, err := ResolveTimersView(true, false, true); err == nil {
		t.Fatal("预期冲突视图报错")
	}
	view, err := ResolveTimersView(false, true, false)
	if err != nil {
		t.Fatalf("解析 raw 视图失败: %v", err)
	}
	if view != TimersViewRaw {
		t.Fatalf("unexpected view: %s", view)
	}
}

func TestBuildServiceRepairHintDeduplicatesUnits(t *testing.T) {
	hint := buildServiceRepairHint([]string{"ccclaw-run.service", "ccclaw-run.service", "ccclaw-journal.service"})
	if count := strings.Count(hint, "ccclaw-run.service"); count != 2 {
		t.Fatalf("expected deduplicated service names in hint, got count=%d hint=%q", count, hint)
	}
	if !strings.Contains(hint, "journalctl --user -u ccclaw-run.service -u ccclaw-journal.service") {
		t.Fatalf("unexpected hint: %q", hint)
	}
}
