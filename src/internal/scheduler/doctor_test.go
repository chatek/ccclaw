package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/41490/ccclaw/internal/config"
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

func TestDoctorJSONReturnsPayloadOnFailure(t *testing.T) {
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	journalctl := filepath.Join(fakeBin, "journalctl")
	if err := os.WriteFile(journalctl, []byte("#!/usr/bin/env bash\nset -euo pipefail\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin)

	cfg := testSystemdConfig()
	cfg.Scheduler.Mode = "none"
	cfg.Scheduler.SystemdUserDir = filepath.Join(dir, "systemd-user")
	cfg.Scheduler.Logs.Level = "bogus"
	cfg.GitHub.ControlRepo = config.OfficialControlRepo
	cfg.Executor.Command = []string{"claude"}
	cfg.Approval.Words = []string{"ok"}
	cfg.Approval.RejectWords = []string{"no"}
	cfg.Approval.MinimumPermission = "maintain"

	var out bytes.Buffer
	err := Doctor(context.Background(), cfg, &out, true)
	if err == nil {
		t.Fatal("预期 doctor 返回失败")
	}
	var payload struct {
		Summary struct {
			Total  int `json:"total"`
			Passed int `json:"passed"`
			Failed int `json:"failed"`
		} `json:"summary"`
		Checks []struct {
			Name  string `json:"name"`
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("解析 JSON 失败: %v; output=%q", err, out.String())
	}
	if payload.Summary.Failed != 2 || payload.Summary.Total == 0 {
		t.Fatalf("unexpected summary: %+v", payload.Summary)
	}
	foundConfig := false
	foundLevel := false
	for _, check := range payload.Checks {
		switch check.Name {
		case "调度配置":
			foundConfig = !check.OK && strings.Contains(check.Error, "scheduler.logs.level")
		case "运行态日志级别":
			foundLevel = !check.OK && strings.Contains(check.Error, "未知运行态日志级别")
		}
	}
	if !foundConfig || !foundLevel {
		t.Fatalf("unexpected checks: %+v", payload.Checks)
	}
}
