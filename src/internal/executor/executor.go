package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/41490/ccclaw/internal/core"
	"github.com/41490/ccclaw/internal/tmux"
)

type Result struct {
	Output      string
	ExitCode    int
	Duration    time.Duration
	LogFile     string
	ResultFile  string
	SessionID   string
	SessionName string
	CostUSD     float64
	Usage       core.TokenUsage
	Pending     bool
}

type Executor struct {
	command     []string
	timeout     time.Duration
	logDir      string
	resultDir   string
	secrets     map[string]string
	tmuxManager tmux.Manager
}

func New(command []string, binary string, timeout time.Duration, logDir, resultDir string, secrets map[string]string, tmuxManager tmux.Manager) (*Executor, error) {
	resolved := make([]string, 0, len(command))
	for _, item := range command {
		if strings.TrimSpace(item) != "" {
			resolved = append(resolved, item)
		}
	}
	if len(resolved) == 0 {
		if binary == "" {
			binary = "claude"
		}
		resolved = []string{binary}
	}
	if _, err := exec.LookPath(resolved[0]); err != nil {
		return nil, fmt.Errorf("找不到执行器命令 %q: %w", resolved[0], err)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建结果目录失败: %w", err)
	}
	return &Executor{
		command:     resolved,
		timeout:     timeout,
		logDir:      logDir,
		resultDir:   resultDir,
		secrets:     copyMap(secrets),
		tmuxManager: tmuxManager,
	}, nil
}

func (e *Executor) Run(parent context.Context, repoPath, taskID, prompt string) (*Result, error) {
	if e.tmuxManager != nil {
		return e.launchTMux(repoPath, taskID, prompt)
	}
	return e.runSync(parent, repoPath, taskID, prompt)
}

func (e *Executor) ArtifactPaths(taskID string) Result {
	safe := safeTaskID(taskID)
	return Result{
		SessionName: sessionName(taskID),
		LogFile:     filepath.Join(e.logDir, safe+".log"),
		ResultFile:  filepath.Join(e.resultDir, safe+".json"),
	}
}

func (e *Executor) LoadResult(taskID string) (*Result, error) {
	artifacts := e.ArtifactPaths(taskID)
	payload, err := os.ReadFile(artifacts.ResultFile)
	if err != nil {
		return nil, fmt.Errorf("读取结果文件失败: %w", err)
	}
	result, parseErr := parseClaudeResult(payload, 0, artifacts.LogFile, artifacts.ResultFile)
	if result != nil {
		result.SessionName = artifacts.SessionName
		result.LogFile = artifacts.LogFile
		result.ResultFile = artifacts.ResultFile
	}
	return result, parseErr
}

func (e *Executor) Timeout() time.Duration {
	return e.timeout
}

func (e *Executor) TMux() tmux.Manager {
	return e.tmuxManager
}

func (e *Executor) launchTMux(repoPath, taskID, prompt string) (*Result, error) {
	artifacts := e.ArtifactPaths(taskID)
	if err := os.Remove(artifacts.LogFile); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("清理旧日志失败: %w", err)
	}
	if err := os.Remove(artifacts.ResultFile); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("清理旧结果失败: %w", err)
	}
	launchCommand := e.buildTMuxCommand(prompt, artifacts.ResultFile)
	if err := e.tmuxManager.Launch(tmux.SessionSpec{
		Name:    artifacts.SessionName,
		WorkDir: repoPath,
		Command: launchCommand,
		LogFile: artifacts.LogFile,
	}); err != nil {
		return nil, fmt.Errorf("启动 tmux 会话失败: %w", err)
	}
	artifacts.Pending = true
	return &artifacts, nil
}

func (e *Executor) buildTMuxCommand(prompt, resultFile string) string {
	args := append([]string{}, e.command...)
	args = append(args,
		"-p", prompt,
		"--dangerously-skip-permissions",
		"--output-format", "json",
	)
	commandLine := shellJoin(args)
	script := fmt.Sprintf("set -o pipefail; %s | tee %s", commandLine, shellQuote(resultFile))
	return fmt.Sprintf("bash -lc %s", shellQuote(script))
}

func (e *Executor) runSync(parent context.Context, repoPath, taskID, prompt string) (*Result, error) {
	ctx, cancel := context.WithTimeout(parent, e.timeout)
	defer cancel()

	artifacts := e.ArtifactPaths(taskID)
	logFile := filepath.Join(e.logDir, fmt.Sprintf("%s_%d.log", safeTaskID(taskID), time.Now().Unix()))
	logHandle, err := os.Create(logFile)
	if err != nil {
		return nil, fmt.Errorf("创建日志文件失败: %w", err)
	}
	defer logHandle.Close()

	args := append([]string{}, e.command[1:]...)
	args = append(args,
		"-p", prompt,
		"--dangerously-skip-permissions",
		"--output-format", "json",
	)
	cmd := exec.CommandContext(ctx, e.command[0], args...)
	cmd.Dir = repoPath
	cmd.Env = append(cmd.Environ(), mapToEnv(e.secrets)...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = io.MultiWriter(logHandle, &stdout)
	cmd.Stderr = io.MultiWriter(logHandle, &stderr)

	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start)
	exitCode := 0
	result, parseErr := parseClaudeResult(stdout.Bytes(), duration, logFile, artifacts.ResultFile)
	if runErr != nil {
		exitCode = 1
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		if result == nil {
			result = &Result{Output: strings.TrimSpace(stdout.String() + stderr.String()), Duration: duration, LogFile: logFile}
		}
		result.ExitCode = exitCode
		if parseErr != nil {
			return result, fmt.Errorf("执行器运行失败: %w；结果校验失败: %v", runErr, parseErr)
		}
		return result, fmt.Errorf("执行器运行失败: %w", runErr)
	}
	if parseErr != nil {
		if result != nil {
			result.ExitCode = exitCode
			return result, parseErr
		}
		return &Result{
			Output:   strings.TrimSpace(stdout.String() + stderr.String()),
			ExitCode: exitCode,
			Duration: duration,
			LogFile:  logFile,
		}, fmt.Errorf("解析 Claude JSON 输出失败: %w", parseErr)
	}
	result.ExitCode = exitCode
	return result, nil
}

type claudeJSONResult struct {
	Type         string          `json:"type"`
	Subtype      string          `json:"subtype"`
	SessionID    string          `json:"session_id"`
	IsError      bool            `json:"is_error"`
	DurationMS   int64           `json:"duration_ms"`
	Result       string          `json:"result"`
	TotalCostUSD float64         `json:"total_cost_usd"`
	Usage        core.TokenUsage `json:"usage"`
}

func parseClaudeResult(raw []byte, duration time.Duration, logFile, resultFile string) (*Result, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("stdout 为空")
	}
	var payload claudeJSONResult
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return nil, err
	}
	result := &Result{
		Output:     payload.Result,
		Duration:   duration,
		LogFile:    logFile,
		ResultFile: resultFile,
		SessionID:  payload.SessionID,
		CostUSD:    payload.TotalCostUSD,
		Usage:      payload.Usage,
	}
	if payload.DurationMS > 0 {
		result.Duration = time.Duration(payload.DurationMS) * time.Millisecond
	}
	if payload.IsError || strings.HasPrefix(payload.Subtype, "error_") {
		if payload.Subtype == "" {
			return result, fmt.Errorf("Claude 返回错误结果")
		}
		return result, fmt.Errorf("Claude 返回错误结果: %s", payload.Subtype)
	}
	if payload.Type != "" && payload.Type != "result" {
		return result, fmt.Errorf("Claude 返回了非 result 类型: %s", payload.Type)
	}
	if result.SessionID == "" && result.Output == "" && result.CostUSD == 0 {
		return result, fmt.Errorf("Claude JSON 输出缺少有效结果")
	}
	return result, nil
}

func SessionName(taskID string) string {
	return sessionName(taskID)
}

func sessionName(taskID string) string {
	checksum := 0
	for idx, r := range taskID {
		checksum += (idx + 1) * int(r)
	}
	return fmt.Sprintf("ccclaw-%s-%04x", safeTaskID(taskID), checksum&0xffff)
}

func safeTaskID(taskID string) string {
	replacer := strings.NewReplacer("#", "_", "/", "_", ":", "_", ".", "_", " ", "_")
	return replacer.Replace(taskID)
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func ParseExitCode(text string) int {
	code, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return 0
	}
	return code
}

func mapToEnv(values map[string]string) []string {
	pairs := make([]string, 0, len(values))
	for key, value := range values {
		pairs = append(pairs, key+"="+value)
	}
	return pairs
}

func copyMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
