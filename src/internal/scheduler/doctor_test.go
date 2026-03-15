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

	if err := os.Remove(filepath.Join(cfg.Scheduler.SystemdUserDir, "ccclaw-ingest.timer")); err != nil {
		t.Fatalf("删除 unit 失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.Scheduler.SystemdUserDir, "ccclaw-patrol.service"), []byte("[Unit]\nDescription=drifted\n"), 0o644); err != nil {
		t.Fatalf("写入漂移 unit 失败: %v", err)
	}

	status, err := DetectManagedUnitDrift(cfg)
	if err != nil {
		t.Fatalf("检测 unit 漂移失败: %v", err)
	}
	if len(status.Missing) != 1 || status.Missing[0] != "ccclaw-ingest.timer" {
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

func TestDoctorJSONWarnsOnLegacyRunUnits(t *testing.T) {
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	systemctl := filepath.Join(fakeBin, "systemctl")
	systemctlBody := `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "--user" && "${2:-}" == "show-environment" ]]; then
  printf 'XDG_RUNTIME_DIR=/run/user/1000\n'
  exit 0
fi
if [[ "${1:-}" == "--user" && "${2:-}" == "show" ]]; then
  unit="${3:-}"
  if [[ "$unit" == *.service ]]; then
    cat <<EOF
Id=${unit}
ActiveState=inactive
SubState=dead
UnitFileState=static
Result=success
ExecMainCode=exited
ExecMainStatus=0
EOF
    exit 0
  fi
  cat <<EOF
Id=${unit}
ActiveState=active
UnitFileState=enabled
NextElapseUSecRealtime=Wed 2026-03-11 06:15:00 EDT
LastTriggerUSec=Wed 2026-03-11 06:10:15 EDT
Result=success
Triggers=${unit%.timer}.service
EOF
  exit 0
fi
if [[ "${1:-}" == "--user" && ( "${2:-}" == "is-enabled" || "${2:-}" == "is-active" ) ]]; then
  printf 'enabled\n'
  exit 0
fi
printf 'unsupported systemctl args: %s\n' "$*" >&2
exit 1
`
	if err := os.WriteFile(systemctl, []byte(systemctlBody), 0o755); err != nil {
		t.Fatal(err)
	}
	loginctl := filepath.Join(fakeBin, "loginctl")
	loginctlBody := `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "show-user" ]]; then
  cat <<EOF
Linger=yes
State=active
EOF
  exit 0
fi
printf 'unsupported loginctl args: %s\n' "$*" >&2
exit 1
`
	if err := os.WriteFile(loginctl, []byte(loginctlBody), 0o755); err != nil {
		t.Fatal(err)
	}
	journalctl := filepath.Join(fakeBin, "journalctl")
	if err := os.WriteFile(journalctl, []byte("#!/usr/bin/env bash\nset -euo pipefail\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfg := testSystemdConfig()
	cfg.Scheduler.SystemdUserDir = filepath.Join(dir, "systemd-user")
	cfg.GitHub.ControlRepo = config.OfficialControlRepo
	cfg.Executor.Command = []string{"claude"}
	cfg.Approval.Words = []string{"ok"}
	cfg.Approval.RejectWords = []string{"no"}
	cfg.Approval.MinimumPermission = "maintain"
	if err := os.MkdirAll(cfg.Scheduler.SystemdUserDir, 0o755); err != nil {
		t.Fatal(err)
	}
	units, err := GenerateSystemdUnitContents(cfg)
	if err != nil {
		t.Fatalf("生成 unit 失败: %v", err)
	}
	for name, content := range units {
		if err := os.WriteFile(filepath.Join(cfg.Scheduler.SystemdUserDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("写入 unit 失败: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(cfg.Scheduler.SystemdUserDir, "ccclaw-run.timer"), []byte("[Unit]\nDescription=legacy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.Scheduler.SystemdUserDir, "ccclaw-run.service"), []byte("[Unit]\nDescription=legacy\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := Doctor(context.Background(), cfg, &out, true); err != nil {
		t.Fatalf("预期 legacy run unit 仅产生告警，实际失败: %v", err)
	}
	var payload struct {
		Summary struct {
			Total  int `json:"total"`
			Passed int `json:"passed"`
			Warned int `json:"warned"`
			Failed int `json:"failed"`
		} `json:"summary"`
		Checks []struct {
			Name    string `json:"name"`
			Level   string `json:"level"`
			OK      bool   `json:"ok"`
			Warning bool   `json:"warning"`
			Repair  string `json:"repair"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("解析 JSON 失败: %v; output=%q", err, out.String())
	}
	if payload.Summary.Warned != 1 || payload.Summary.Failed != 0 {
		t.Fatalf("unexpected summary: %+v", payload.Summary)
	}
	foundLegacy := false
	for _, check := range payload.Checks {
		if check.Name != "遗留 run 单元" {
			continue
		}
		foundLegacy = true
		if !check.OK || !check.Warning || check.Level != "warn" {
			t.Fatalf("unexpected legacy check: %+v", check)
		}
		if !strings.Contains(check.Repair, "ccclaw-run.timer") {
			t.Fatalf("unexpected repair: %+v", check)
		}
	}
	if !foundLegacy {
		t.Fatalf("missing legacy run check: %+v", payload.Checks)
	}
}
