package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/41490/ccclaw/internal/config"
	"github.com/41490/ccclaw/internal/scheduler"
)

func TestRootCmdVersionFlag(t *testing.T) {
	cmd := newRootCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"-V"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("执行 -V 失败: %v", err)
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Fatal("预期输出版本号，实际为空")
	}
}

func TestRootCmdHelpByDefault(t *testing.T) {
	cmd := newRootCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("执行默认帮助失败: %v", err)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("预期输出帮助信息，实际为: %q", out.String())
	}
}

func TestConfigMigrateApprovalCommand(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := `[github]
control_repo = "41490/ccclaw"

[paths]
app_dir = "~/.ccclaw"
home_repo = "/opt/ccclaw"
state_db = "~/.ccclaw/var/state.db"
log_dir = "~/.ccclaw/log"
kb_dir = "/opt/ccclaw/kb"
env_file = "~/.ccclaw/.env"

[executor]
command = ["claude"]

[approval]
command = "/ccclaw approve"
minimum_permission = "admin"
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--config", configPath, "config", "migrate-approval"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("执行 config migrate-approval 失败: %v", err)
	}
	if !strings.Contains(out.String(), "已迁移审批配置") {
		t.Fatalf("unexpected output: %q", out.String())
	}

	payload, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), `command = "/ccclaw approve"`) {
		t.Fatalf("legacy command should be removed: %q", string(payload))
	}
}

func TestConfigSetSchedulerCommand(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := `default_target = ""

[github]
control_repo = "41490/ccclaw"

[paths]
app_dir = "/tmp/ccclaw-app"
home_repo = "/opt/ccclaw"
state_db = "/tmp/ccclaw-app/var/state.db"
log_dir = "/tmp/ccclaw-app/log"
kb_dir = "/opt/ccclaw/kb"
env_file = "/tmp/ccclaw-app/.env"

[executor]
command = ["claude"]

[scheduler]
mode = "none"
systemd_user_dir = "/tmp/systemd-old"

[approval]
words = ["approve"]
reject_words = ["reject"]
minimum_permission = "maintain"
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--config", configPath, "config", "set-scheduler", "--mode", "cron", "--systemd-user-dir", "/tmp/systemd-new"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("执行 config set-scheduler 失败: %v", err)
	}
	if !strings.Contains(out.String(), "已更新调度配置") {
		t.Fatalf("unexpected output: %q", out.String())
	}

	payload, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	if !strings.Contains(text, "mode = 'cron'") {
		t.Fatalf("expected scheduler mode update: %q", text)
	}
	if !strings.Contains(text, "systemd_user_dir = '/tmp/systemd-new'") {
		t.Fatalf("expected systemd dir update: %q", text)
	}
}

func TestSchedulerStatusCommand(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	envPath := filepath.Join(dir, ".env")
	fakeBin := filepath.Join(dir, "bin")
	crontabStore := filepath.Join(dir, "crontab.txt")
	systemctlLog := filepath.Join(dir, "systemctl.log")
	if err := os.WriteFile(envPath, []byte("GH_TOKEN=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(testConfigToml(dir, envPath, "none")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFakeCrontab(t, filepath.Join(fakeBin, "crontab"), crontabStore)
	writeFakeSystemctlProbe(t, filepath.Join(fakeBin, "systemctl"), systemctlLog)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CCCLAW_FAKE_CRONTAB_FILE", crontabStore)
	t.Setenv("CCCLAW_FAKE_SYSTEMCTL_LOG", systemctlLog)

	cmd := newRootCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--config", configPath, "--env-file", envPath, "scheduler", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("执行 scheduler status 失败: %v", err)
	}
	if !strings.Contains(out.String(), "request=none effective=none") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestSchedulerUseCronCommand(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("GH_TOKEN=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	appDir := filepath.Join(dir, "app")
	systemdDir := filepath.Join(dir, "systemd-user")
	if err := os.MkdirAll(filepath.Join(appDir, "ops", "systemd"), 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte(testConfigTomlWithSystemdDir(appDir, envPath, systemdDir, "systemd")), 0o644); err != nil {
		t.Fatal(err)
	}

	fakeBin := filepath.Join(dir, "bin")
	crontabStore := filepath.Join(dir, "crontab.txt")
	systemctlLog := filepath.Join(dir, "systemctl.log")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFakeCrontab(t, filepath.Join(fakeBin, "crontab"), crontabStore)
	writeFakeSystemctl(t, filepath.Join(fakeBin, "systemctl"), systemctlLog)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CCCLAW_FAKE_CRONTAB_FILE", crontabStore)
	t.Setenv("CCCLAW_FAKE_SYSTEMCTL_LOG", systemctlLog)

	cmd := newRootCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--config", configPath, "scheduler", "use", "cron"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("执行 scheduler use cron 失败: %v", err)
	}

	payload, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "mode = 'cron'") {
		t.Fatalf("expected mode=cron: %q", string(payload))
	}
	cronPayload, err := os.ReadFile(crontabStore)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cronPayload), scheduler.ManagedCronBegin) {
		t.Fatalf("expected managed cron block: %q", string(cronPayload))
	}
	logPayload, err := os.ReadFile(systemctlLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logPayload), "disable --now ccclaw-ingest.timer") {
		t.Fatalf("expected systemctl disable call: %q", string(logPayload))
	}
	if !strings.Contains(out.String(), "已切换调度后端: mode=cron") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestSchedulerUseSystemdCommand(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("GH_TOKEN=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	appDir := filepath.Join(dir, "app")
	systemdDir := filepath.Join(dir, "systemd-user")
	sourceDir := filepath.Join(appDir, "ops", "systemd")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"ccclaw-ingest.service",
		"ccclaw-ingest.timer",
		"ccclaw-run.service",
		"ccclaw-run.timer",
		"ccclaw-patrol.service",
		"ccclaw-patrol.timer",
		"ccclaw-journal.service",
		"ccclaw-journal.timer",
	} {
		if err := os.WriteFile(filepath.Join(sourceDir, name), []byte("[Unit]\nDescription=test\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte(testConfigTomlWithSystemdDir(appDir, envPath, systemdDir, "cron")), 0o644); err != nil {
		t.Fatal(err)
	}

	fakeBin := filepath.Join(dir, "bin")
	crontabStore := filepath.Join(dir, "crontab.txt")
	systemctlLog := filepath.Join(dir, "systemctl.log")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFakeCrontab(t, filepath.Join(fakeBin, "crontab"), crontabStore)
	if err := os.WriteFile(crontabStore, []byte(scheduler.ManagedCronBlock(testSchedulerConfig(appDir, envPath, systemdDir, "cron").toConfig())), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFakeSystemctl(t, filepath.Join(fakeBin, "systemctl"), systemctlLog)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CCCLAW_FAKE_CRONTAB_FILE", crontabStore)
	t.Setenv("CCCLAW_FAKE_SYSTEMCTL_LOG", systemctlLog)

	cmd := newRootCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--config", configPath, "scheduler", "use", "systemd"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("执行 scheduler use systemd 失败: %v", err)
	}

	payload, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "mode = 'systemd'") {
		t.Fatalf("expected mode=systemd: %q", string(payload))
	}
	cronPayload, err := os.ReadFile(crontabStore)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cronPayload), scheduler.ManagedCronBegin) {
		t.Fatalf("managed cron should be removed: %q", string(cronPayload))
	}
	if _, err := os.Stat(filepath.Join(systemdDir, "ccclaw-ingest.service")); err != nil {
		t.Fatalf("expected unit file copied: %v", err)
	}
	logPayload, err := os.ReadFile(systemctlLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logPayload), "enable --now ccclaw-ingest.timer") {
		t.Fatalf("expected systemctl enable call: %q", string(logPayload))
	}
	if !strings.Contains(out.String(), "已切换调度后端: mode=systemd") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestParseStatsOptions(t *testing.T) {
	options, err := parseStatsOptions("2026-03-09", "2026-03-10", true, true, 7)
	if err != nil {
		t.Fatalf("解析 stats 选项失败: %v", err)
	}
	if !options.Daily || !options.ShowRTKComparison {
		t.Fatalf("unexpected options flags: %#v", options)
	}
	if options.Limit != 7 {
		t.Fatalf("unexpected limit: %#v", options)
	}
	if options.Start.Format("2006-01-02") != "2026-03-09" {
		t.Fatalf("unexpected start: %#v", options.Start)
	}
	if options.End.Format("2006-01-02") != "2026-03-11" {
		t.Fatalf("unexpected end: %#v", options.End)
	}
}

func testConfigToml(root, envPath, mode string) string {
	return testConfigTomlWithSystemdDir(filepath.Join(root, "app"), envPath, filepath.Join(root, "systemd-user"), mode)
}

func testConfigTomlWithSystemdDir(appDir, envPath, systemdDir, mode string) string {
	return "default_target = ''\n\n" +
		"[github]\ncontrol_repo = '41490/ccclaw'\n\n" +
		"[paths]\n" +
		"app_dir = '" + appDir + "'\n" +
		"home_repo = '/opt/ccclaw'\n" +
		"state_db = '" + filepath.Join(appDir, "var", "state.db") + "'\n" +
		"log_dir = '" + filepath.Join(appDir, "log") + "'\n" +
		"kb_dir = '/opt/ccclaw/kb'\n" +
		"env_file = '" + envPath + "'\n\n" +
		"[executor]\ncommand = ['claude']\n\n" +
		"[scheduler]\n" +
		"mode = '" + mode + "'\n" +
		"systemd_user_dir = '" + systemdDir + "'\n\n" +
		"[approval]\n" +
		"words = ['approve']\n" +
		"reject_words = ['reject']\n" +
		"minimum_permission = 'maintain'\n"
}

func testSchedulerConfig(appDir, envPath, systemdDir, mode string) *schedulerConfigFixture {
	return &schedulerConfigFixture{
		AppDir:     appDir,
		EnvPath:    envPath,
		SystemdDir: systemdDir,
		Mode:       mode,
	}
}

type schedulerConfigFixture struct {
	AppDir     string
	EnvPath    string
	SystemdDir string
	Mode       string
}

func (f *schedulerConfigFixture) toConfig() *config.Config {
	return &config.Config{
		GitHub: config.GitHubConfig{ControlRepo: "41490/ccclaw"},
		Paths: config.PathsConfig{
			AppDir:   f.AppDir,
			HomeRepo: "/opt/ccclaw",
			StateDB:  filepath.Join(f.AppDir, "var", "state.db"),
			LogDir:   filepath.Join(f.AppDir, "log"),
			KBDir:    "/opt/ccclaw/kb",
			EnvFile:  f.EnvPath,
		},
		Executor: config.ExecutorConfig{Command: []string{"claude"}},
		Scheduler: config.SchedulerConfig{
			Mode:           f.Mode,
			SystemdUserDir: f.SystemdDir,
		},
		Approval: config.ApprovalConfig{
			Words:             []string{"approve"},
			RejectWords:       []string{"reject"},
			MinimumPermission: "maintain",
		},
	}
}

func writeFakeCrontab(t *testing.T, scriptPath, store string) {
	t.Helper()
	body := `#!/usr/bin/env bash
set -euo pipefail
store="${CCCLAW_FAKE_CRONTAB_FILE:?}"
case "${1:-}" in
  -l)
    if [[ -f "$store" && -s "$store" ]]; then
      cat "$store"
      exit 0
    fi
    printf 'no crontab for tester\n' >&2
    exit 1
    ;;
  -)
    cat > "$store"
    exit 0
    ;;
  *)
    printf 'unsupported crontab args: %s\n' "$*" >&2
    exit 1
    ;;
esac
`
	if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFakeSystemctl(t *testing.T, scriptPath, logPath string) {
	t.Helper()
	body := `#!/usr/bin/env bash
set -euo pipefail
log_file="${CCCLAW_FAKE_SYSTEMCTL_LOG:?}"
printf '%s\n' "$*" >> "$log_file"
exit 0
`
	if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFakeSystemctlProbe(t *testing.T, scriptPath, logPath string) {
	t.Helper()
	body := `#!/usr/bin/env bash
set -euo pipefail
log_file="${CCCLAW_FAKE_SYSTEMCTL_LOG:?}"
printf '%s\n' "$*" >> "$log_file"
printf 'disabled\n' >&2
exit 1
`
	if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseStatsOptionsRejectsInvalidRange(t *testing.T) {
	_, err := parseStatsOptions("2026-03-10", "2026-03-09", false, false, 20)
	if err == nil {
		t.Fatal("预期无效日期范围报错")
	}
}

func TestParseStatsOptionsAcceptsOpenEndedRange(t *testing.T) {
	options, err := parseStatsOptions("", "2026-03-10", false, false, 20)
	if err != nil {
		t.Fatalf("解析开放区间失败: %v", err)
	}
	if !options.Start.IsZero() {
		t.Fatalf("unexpected start: %#v", options.Start)
	}
	want := time.Date(2026, 3, 11, 0, 0, 0, 0, time.Local)
	if !options.End.Equal(want) {
		t.Fatalf("unexpected end: got=%v want=%v", options.End, want)
	}
}

func TestParseStatsOptionsRejectsNonPositiveLimit(t *testing.T) {
	_, err := parseStatsOptions("", "", false, false, 0)
	if err == nil {
		t.Fatal("预期 limit 非法时报错")
	}
}
