package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
