package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/41490/ccclaw/internal/tmux"
)

type fakeTMuxManager struct {
	spec tmux.SessionSpec
}

func (f *fakeTMuxManager) Launch(spec tmux.SessionSpec) error {
	f.spec = spec
	return nil
}

func (f *fakeTMuxManager) Status(name string) (*tmux.SessionStatus, error) { return nil, nil }
func (f *fakeTMuxManager) CaptureOutput(name string, lines int) (string, error) {
	return "", nil
}
func (f *fakeTMuxManager) Kill(name string) error { return nil }
func (f *fakeTMuxManager) List(prefix string) ([]tmux.SessionStatus, error) {
	return nil, nil
}

func TestExecutorRunParsesJSONResult(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "fake-claude.sh")
	script := "#!/bin/sh\ncat <<'EOF'\n{\"type\":\"result\",\"subtype\":\"success\",\"session_id\":\"sess-1\",\"result\":\"任务完成\",\"total_cost_usd\":0.125,\"usage\":{\"input_tokens\":12,\"output_tokens\":8,\"cache_creation_input_tokens\":3,\"cache_read_input_tokens\":4}}\nEOF\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("写入脚本失败: %v", err)
	}
	execEngine, err := New([]string{scriptPath}, "", time.Minute, filepath.Join(tmpDir, "log"), filepath.Join(tmpDir, "result"), nil, nil)
	if err != nil {
		t.Fatalf("创建执行器失败: %v", err)
	}

	result, err := execEngine.Run(context.Background(), tmpDir, "10#body", RunOptions{Prompt: "test prompt"})
	if err != nil {
		t.Fatalf("执行器运行失败: %v", err)
	}
	if result.SessionID != "sess-1" {
		t.Fatalf("unexpected session id: %q", result.SessionID)
	}
	if result.CostUSD != 0.125 {
		t.Fatalf("unexpected cost: %v", result.CostUSD)
	}
	if result.Usage.InputTokens != 12 || result.Usage.OutputTokens != 8 {
		t.Fatalf("unexpected usage: %#v", result.Usage)
	}
	if result.Output != "任务完成" {
		t.Fatalf("unexpected result output: %q", result.Output)
	}
	if result.PromptFile == "" {
		t.Fatalf("预期写入 prompt 归档，实际为 %#v", result)
	}
}

func TestExecutorRunReturnsStructuredErrorResult(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "fake-claude-error.sh")
	script := "#!/bin/sh\ncat <<'EOF'\n{\"type\":\"result\",\"subtype\":\"error_max_turns\",\"session_id\":\"sess-error\",\"result\":\"达到上限\",\"is_error\":true,\"total_cost_usd\":0.5,\"usage\":{\"input_tokens\":20,\"output_tokens\":10}}\nEOF\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("写入脚本失败: %v", err)
	}
	execEngine, err := New([]string{scriptPath}, "", time.Minute, filepath.Join(tmpDir, "log"), filepath.Join(tmpDir, "result"), nil, nil)
	if err != nil {
		t.Fatalf("创建执行器失败: %v", err)
	}

	result, runErr := execEngine.Run(context.Background(), tmpDir, "10#body", RunOptions{Prompt: "test prompt", ResumeSessionID: "sess-error"})
	if runErr == nil {
		t.Fatal("预期返回结构化错误")
	}
	if !strings.Contains(runErr.Error(), "error_max_turns") {
		t.Fatalf("unexpected error: %v", runErr)
	}
	if result == nil || result.SessionID != "sess-error" {
		t.Fatalf("预期保留 session_id，实际为 %#v", result)
	}
	if result.CostUSD != 0.5 {
		t.Fatalf("unexpected cost: %v", result.CostUSD)
	}
	if result.PromptFile == "" {
		t.Fatalf("预期返回 prompt 归档路径，实际为 %#v", result)
	}
}

func TestExecutorRunForcesPlainClaudeMode(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "fake-wrapper.sh")
	script := "#!/bin/sh\nset -eu\ncat <<'EOF'\n{\"type\":\"result\",\"subtype\":\"success\",\"session_id\":\"sess-plain\",\"result\":\"任务完成\",\"total_cost_usd\":0.125,\"usage\":{\"input_tokens\":12,\"output_tokens\":8}}\nEOF\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("写入脚本失败: %v", err)
	}
	execEngine, err := New([]string{scriptPath}, "", time.Minute, filepath.Join(tmpDir, "log"), filepath.Join(tmpDir, "result"), nil, nil)
	if err != nil {
		t.Fatalf("创建执行器失败: %v", err)
	}

	result, err := execEngine.Run(context.Background(), tmpDir, "10#body", RunOptions{Prompt: "test prompt"})
	if err != nil {
		t.Fatalf("执行器运行失败: %v", err)
	}
	if result.RTKEnabled {
		t.Fatalf("expected plain claude mode, got %#v", result)
	}
}

func TestExecutorRunLaunchesTMuxSession(t *testing.T) {
	tmpDir := t.TempDir()
	manager := &fakeTMuxManager{}
	execEngine, err := New([]string{"sh"}, "", time.Minute, filepath.Join(tmpDir, "log"), filepath.Join(tmpDir, "result"), nil, manager)
	if err != nil {
		t.Fatalf("创建执行器失败: %v", err)
	}

	result, runErr := execEngine.Run(context.Background(), tmpDir, "10#body", RunOptions{Prompt: "test prompt", ResumeSessionID: "sess-9"})
	if runErr != nil {
		t.Fatalf("tmux launch 失败: %v", runErr)
	}
	if result == nil || !result.Pending {
		t.Fatalf("预期返回 pending 结果，实际为 %#v", result)
	}
	if !strings.Contains(manager.spec.Command, "--output-format") || !strings.Contains(manager.spec.Command, "json") {
		t.Fatalf("预期 tmux 命令包含 json 输出，实际为 %q", manager.spec.Command)
	}
	if !strings.Contains(manager.spec.Command, "--resume") || !strings.Contains(manager.spec.Command, "sess-9") {
		t.Fatalf("预期 tmux 命令包含 resume 参数，实际为 %q", manager.spec.Command)
	}
	if manager.spec.Name == "" || !strings.HasPrefix(manager.spec.Name, "ccclaw-") {
		t.Fatalf("unexpected session name: %#v", manager.spec)
	}
	if result.PromptFile == "" || result.MetaFile == "" {
		t.Fatalf("预期返回 prompt/meta 路径，实际为 %#v", result)
	}
	if !strings.Contains(manager.spec.Command, "2>&1 | tee") {
		t.Fatalf("预期 tmux 命令同时采集 stderr，实际为 %q", manager.spec.Command)
	}
}

func TestExecutorRuntimeEnvDiscoversLocalBins(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	localBin := filepath.Join(homeDir, ".local", "bin")
	if err := os.MkdirAll(localBin, 0o755); err != nil {
		t.Fatalf("创建 local bin 失败: %v", err)
	}
	for _, name := range []string{"claude", "rtk"} {
		path := filepath.Join(localBin, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("写入 fake %s 失败: %v", name, err)
		}
	}
	wrapperPath := filepath.Join(tmpDir, "ccclaude")
	if err := os.WriteFile(wrapperPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("写入 wrapper 失败: %v", err)
	}
	t.Setenv("HOME", homeDir)
	t.Setenv("PATH", "/usr/bin:/bin")

	execEngine, err := New([]string{wrapperPath}, "", time.Minute, filepath.Join(tmpDir, "log"), filepath.Join(tmpDir, "result"), nil, nil)
	if err != nil {
		t.Fatalf("创建执行器失败: %v", err)
	}

	env := execEngine.runtimeEnv("", "")
	if got, want := env["CCCLAW_CLAUDE_BIN"], filepath.Join(localBin, "claude"); got != want {
		t.Fatalf("unexpected claude bin: got=%q want=%q", got, want)
	}
	if _, ok := env["CCCLAW_RTK_BIN"]; ok {
		t.Fatalf("did not expect rtk bin in runtime env: %#v", env)
	}
}

func TestLoadResultFallsBackToRawOutput(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "claude", raw: "ccclaude: exec: claude: not found\n", want: "claude: not found"},
		{name: "rtk", raw: "ccclaude: exec: /tmp/rtk: not found\n", want: "/tmp/rtk: not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			scriptPath := filepath.Join(tmpDir, "fake-claude.sh")
			if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatalf("写入脚本失败: %v", err)
			}
			execEngine, err := New([]string{scriptPath}, "", time.Minute, filepath.Join(tmpDir, "log"), filepath.Join(tmpDir, "result"), nil, nil)
			if err != nil {
				t.Fatalf("创建执行器失败: %v", err)
			}

			artifacts := execEngine.ArtifactPaths("10#body")
			if err := os.WriteFile(artifacts.ResultFile, []byte(tc.raw), 0o644); err != nil {
				t.Fatalf("写入结果文件失败: %v", err)
			}

			result, loadErr := execEngine.LoadResult("10#body")
			if loadErr == nil {
				t.Fatal("预期返回解析错误")
			}
			if result == nil || !strings.Contains(result.Output, tc.want) {
				t.Fatalf("预期保留原始诊断输出，实际为 %#v", result)
			}
		})
	}
}
