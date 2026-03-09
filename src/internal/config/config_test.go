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
command = "/ccclaw approve"
minimum_permission = "write"

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
