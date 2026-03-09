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
