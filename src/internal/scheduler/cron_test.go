package scheduler

import (
	"strings"
	"testing"

	"github.com/41490/ccclaw/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Paths: config.PathsConfig{
			AppDir:  "/tmp/ccclaw-app",
			EnvFile: "/tmp/ccclaw-app/.env",
		},
	}
}

func TestManagedCronBlockIncludesAllCommands(t *testing.T) {
	cfg := testConfig()
	block := ManagedCronBlock(cfg)
	for _, command := range ManagedCronCommands(cfg) {
		if !strings.Contains(block, command) {
			t.Fatalf("缺少命令: %s", command)
		}
	}
	if !strings.Contains(block, ManagedCronBegin) || !strings.Contains(block, ManagedCronEnd) {
		t.Fatalf("缺少受控边界: %q", block)
	}
}

func TestStripManagedCronPreservesUserRules(t *testing.T) {
	cfg := testConfig()
	content := strings.Join([]string{
		"MAILTO=test@example.com",
		ManagedCronBegin,
		"",
	}, "\n")
	for _, command := range ManagedCronCommands(cfg) {
		content += command + "\n"
	}
	content += strings.Join([]string{
		ManagedCronEnd,
		"15 4 * * * echo keep-me",
		"",
	}, "\n")
	trimmed, found := StripManagedCron(content)
	if !found {
		t.Fatal("预期识别到受控块")
	}
	if strings.Contains(trimmed, ManagedCronBegin) || strings.Contains(trimmed, ManagedCronEnd) {
		t.Fatalf("受控边界未被移除: %q", trimmed)
	}
	if !strings.Contains(trimmed, "MAILTO=test@example.com") || !strings.Contains(trimmed, "keep-me") {
		t.Fatalf("用户规则应被保留: %q", trimmed)
	}
}

func TestContainsManagedCronRequiresFullSet(t *testing.T) {
	cfg := testConfig()
	block := ManagedCronBlock(cfg)
	if !ContainsManagedCron(block, cfg) {
		t.Fatal("预期完整受控块被识别")
	}
	commands := ManagedCronCommands(cfg)
	partial := strings.Replace(block, commands[len(commands)-1], "", 1)
	if ContainsManagedCron(partial, cfg) {
		t.Fatalf("缺少 journal 规则时不应视为完整: %q", partial)
	}
}
