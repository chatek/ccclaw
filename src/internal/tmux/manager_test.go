package tmux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecManagerLaunchUsesDirectNewSessionCommand(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "tmux.log")
	tmuxBin := filepath.Join(tmpDir, "tmux")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + logFile + "\"\nexit 0\n"
	if err := os.WriteFile(tmuxBin, []byte(script), 0o755); err != nil {
		t.Fatalf("写入 fake tmux 失败: %v", err)
	}

	manager, err := New(tmuxBin)
	if err != nil {
		t.Fatalf("创建 tmux manager 失败: %v", err)
	}
	spec := SessionSpec{
		Name:    "ccclaw-test",
		WorkDir: tmpDir,
		Command: "bash -lc 'echo ok'",
		LogFile: filepath.Join(tmpDir, "pane.log"),
	}
	if err := manager.Launch(spec); err != nil {
		t.Fatalf("启动 tmux 会话失败: %v", err)
	}

	payload, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("读取 tmux 调用日志失败: %v", err)
	}
	text := string(payload)
	if !strings.Contains(text, "new-session -d -s ccclaw-test -c "+tmpDir+" bash -lc 'echo ok'") {
		t.Fatalf("预期 new-session 直接携带命令，实际为 %q", text)
	}
	if strings.Contains(text, "send-keys") {
		t.Fatalf("不应再调用 send-keys，实际为 %q", text)
	}
	if strings.Contains(text, "pipe-pane") {
		t.Fatalf("不应再依赖 pipe-pane，实际为 %q", text)
	}
}

func TestExecManagerListFiltersBeforeParsingDeadStatus(t *testing.T) {
	tmpDir := t.TempDir()
	tmuxBin := filepath.Join(tmpDir, "tmux")
	script := `#!/bin/sh
set -eu
if [ "${1:-}" = "list-panes" ]; then
  printf 'other-session\t1710000000\t0\t\t123\n'
  printf 'ccclaw-test\t1710000100\t1\t0\t456\n'
  exit 0
fi
exit 1
`
	if err := os.WriteFile(tmuxBin, []byte(script), 0o755); err != nil {
		t.Fatalf("写入 fake tmux 失败: %v", err)
	}

	manager, err := New(tmuxBin)
	if err != nil {
		t.Fatalf("创建 tmux manager 失败: %v", err)
	}
	items, err := manager.List("ccclaw-")
	if err != nil {
		t.Fatalf("列出 tmux 会话失败: %v", err)
	}
	if len(items) != 1 || items[0].Name != "ccclaw-test" || !items[0].PaneDead || items[0].ExitCode != 0 {
		t.Fatalf("unexpected tmux items: %#v", items)
	}
}
