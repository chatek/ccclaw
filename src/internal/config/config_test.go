package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("GH_TOKEN=test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEnvFile(path); err != nil {
		t.Fatalf("expected valid env file, got %v", err)
	}

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEnvFile(path); err == nil {
		t.Fatal("expected permission validation error")
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got := ExpandPath("~/.ccclaw")
	want := filepath.Join(home, ".ccclaw")
	if got != want {
		t.Fatalf("unexpected expanded path: got %q want %q", got, want)
	}
}

func TestLoadConfigWithCommandAndHomePaths(t *testing.T) {
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
provider = "claude-code"
command = ["~/.ccclaw/bin/ccclaude"]
timeout = "30m"

[approval]
words = ["approve", "go", "推进"]
reject_words = ["reject", "no", "拒绝"]
minimum_permission = "maintain"

[[targets]]
repo = "41490/ccclaw"
local_path = "~/src/ccclaw"
kb_path = "/opt/ccclaw/kb"
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Executor.Command) != 1 {
		t.Fatalf("unexpected executor command: %#v", cfg.Executor.Command)
	}
	if cfg.Executor.Command[0] == "~/.ccclaw/bin/ccclaude" {
		t.Fatal("expected executor command path to be expanded")
	}
	if cfg.Targets[0].LocalPath == "~/src/ccclaw" {
		t.Fatal("expected target local path to be expanded")
	}
}

func TestLoadConfigAllowsZeroTargets(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := `default_target = ""

[github]
control_repo = "41490/ccclaw"

[paths]
app_dir = "~/.ccclaw"
home_repo = "/opt/ccclaw"
state_db = "~/.ccclaw/var/state.db"
log_dir = "~/.ccclaw/log"
kb_dir = "/opt/ccclaw/kb"
env_file = "~/.ccclaw/.env"

[executor]
provider = "claude-code"
command = ["~/.ccclaw/bin/ccclaude"]
timeout = "30m"

[approval]
words = ["approve"]
reject_words = ["reject"]
minimum_permission = "maintain"
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Targets) != 0 {
		t.Fatalf("expected no targets, got %d", len(cfg.Targets))
	}
}

func TestLoadConfigRejectsLegacyApprovalCommandField(t *testing.T) {
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
provider = "claude-code"
command = ["~/.ccclaw/bin/ccclaude"]
timeout = "30m"

[approval]
command = "/ccclaw approve"
minimum_permission = "maintain"
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(configPath); err == nil {
		t.Fatal("expected legacy approval.command to be rejected")
	}
}

func TestDisableTargetClearsDefaultTarget(t *testing.T) {
	cfg := &Config{
		DefaultTarget: "41490/ccclaw",
		GitHub:        GitHubConfig{ControlRepo: "41490/ccclaw"},
		Paths: PathsConfig{
			AppDir:   "/tmp/app",
			HomeRepo: "/opt/ccclaw",
			StateDB:  "/tmp/app/var/state.db",
			LogDir:   "/tmp/app/log",
			KBDir:    "/opt/ccclaw/kb",
			EnvFile:  "/tmp/app/.env",
		},
		Executor: ExecutorConfig{Command: []string{"claude"}, Timeout: "30m"},
		Approval: ApprovalConfig{
			Words:             []string{"approve", "go"},
			RejectWords:       []string{"reject"},
			MinimumPermission: "maintain",
		},
		Targets: []TargetConfig{{
			Repo:      "41490/ccclaw",
			LocalPath: "/opt/src/ccclaw",
			KBPath:    "/opt/ccclaw/kb",
		}},
	}
	if err := cfg.DisableTarget("41490/ccclaw"); err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultTarget != "" {
		t.Fatalf("expected default target to be cleared, got %q", cfg.DefaultTarget)
	}
	if !cfg.Targets[0].Disabled {
		t.Fatal("expected target to be disabled")
	}
}
