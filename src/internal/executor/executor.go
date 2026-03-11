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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/41490/ccclaw/internal/core"
	"github.com/41490/ccclaw/internal/tmux"
)

type Result struct {
	Output          string
	ExitCode        int
	Duration        time.Duration
	LogFile         string
	ResultFile      string
	MetaFile        string
	PromptFile      string
	SessionID       string
	SessionName     string
	CostUSD         float64
	Usage           core.TokenUsage
	RTKEnabled      bool
	ResumeSessionID string
	Pending         bool
}

type RunOptions struct {
	Prompt          string
	ResumeSessionID string
}

type Executor struct {
	command     []string
	timeout     time.Duration
	logDir      string
	resultDir   string
	promptDir   string
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
	promptDir := filepath.Join(filepath.Dir(resultDir), "prompts")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建 prompt 归档目录失败: %w", err)
	}
	return &Executor{
		command:     resolved,
		timeout:     timeout,
		logDir:      logDir,
		resultDir:   resultDir,
		promptDir:   promptDir,
		secrets:     copyMap(secrets),
		tmuxManager: tmuxManager,
	}, nil
}

func (e *Executor) Run(parent context.Context, repoPath, taskID string, opts RunOptions) (*Result, error) {
	if e.tmuxManager != nil {
		return e.launchTMux(repoPath, taskID, opts)
	}
	return e.runSync(parent, repoPath, taskID, opts)
}

func (e *Executor) ArtifactPaths(taskID string) Result {
	safe := safeTaskID(taskID)
	return Result{
		SessionName: sessionName(taskID),
		LogFile:     filepath.Join(e.logDir, safe+".log"),
		ResultFile:  filepath.Join(e.resultDir, safe+".json"),
		MetaFile:    filepath.Join(e.resultDir, safe+".meta.json"),
	}
}

func (e *Executor) LoadResult(taskID string) (*Result, error) {
	artifacts := e.ArtifactPaths(taskID)
	meta, metaErr := e.loadRunMetadata(artifacts.MetaFile)
	if metaErr != nil {
		return nil, metaErr
	}
	payload, err := os.ReadFile(artifacts.ResultFile)
	if err != nil {
		return nil, fmt.Errorf("读取结果文件失败: %w", err)
	}
	result, parseErr := parseClaudeResult(payload, 0, artifacts.LogFile, artifacts.ResultFile)
	if result != nil {
		e.applyArtifactMetadata(result, artifacts, meta)
	}
	return result, parseErr
}

func (e *Executor) Timeout() time.Duration {
	return e.timeout
}

func (e *Executor) TMux() tmux.Manager {
	return e.tmuxManager
}

func (e *Executor) launchTMux(repoPath, taskID string, opts RunOptions) (*Result, error) {
	artifacts, meta, err := e.prepareArtifacts(taskID, opts)
	if err != nil {
		return nil, err
	}
	if err := os.Remove(artifacts.LogFile); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("清理旧日志失败: %w", err)
	}
	if err := os.Remove(artifacts.ResultFile); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("清理旧结果失败: %w", err)
	}
	if err := os.Remove(artifacts.MetaFile); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("清理旧元数据失败: %w", err)
	}
	if strings.TrimSpace(meta.RTKMarkerFile) != "" {
		if err := os.Remove(meta.RTKMarkerFile); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("清理旧 rtk 标记失败: %w", err)
		}
	}
	if err := e.writeRunMetadata(artifacts.MetaFile, meta); err != nil {
		return nil, err
	}
	launchCommand := e.buildTMuxCommand(opts, artifacts.ResultFile, meta.RTKMarkerFile)
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

func (e *Executor) buildTMuxCommand(opts RunOptions, resultFile, rtkMarkerFile string) string {
	commandLine := shellJoin(e.commandWithEnv(opts, rtkMarkerFile))
	script := fmt.Sprintf("set -o pipefail; %s | tee %s", commandLine, shellQuote(resultFile))
	return fmt.Sprintf("bash -lc %s", shellQuote(script))
}

func (e *Executor) runSync(parent context.Context, repoPath, taskID string, opts RunOptions) (*Result, error) {
	ctx, cancel := context.WithTimeout(parent, e.timeout)
	defer cancel()

	artifacts, meta, err := e.prepareArtifacts(taskID, opts)
	if err != nil {
		return nil, err
	}
	if err := os.Remove(artifacts.LogFile); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("清理旧日志失败: %w", err)
	}
	if err := os.Remove(artifacts.ResultFile); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("清理旧结果失败: %w", err)
	}
	if err := os.Remove(artifacts.MetaFile); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("清理旧元数据失败: %w", err)
	}
	if strings.TrimSpace(meta.RTKMarkerFile) != "" {
		if err := os.Remove(meta.RTKMarkerFile); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("清理旧 rtk 标记失败: %w", err)
		}
	}
	if err := e.writeRunMetadata(artifacts.MetaFile, meta); err != nil {
		return nil, err
	}

	logHandle, err := os.Create(artifacts.LogFile)
	if err != nil {
		return nil, fmt.Errorf("创建日志文件失败: %w", err)
	}
	defer logHandle.Close()

	args := e.commandArgs(opts)
	cmd := exec.CommandContext(ctx, e.command[0], args...)
	cmd.Dir = repoPath
	cmd.Env = append(cmd.Environ(), mapToEnv(e.runtimeEnv(meta.RTKMarkerFile))...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = io.MultiWriter(logHandle, &stdout)
	cmd.Stderr = io.MultiWriter(logHandle, &stderr)

	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start)
	exitCode := 0
	if writeErr := os.WriteFile(artifacts.ResultFile, stdout.Bytes(), 0o644); writeErr != nil {
		return nil, fmt.Errorf("写入结果文件失败: %w", writeErr)
	}
	result, parseErr := parseClaudeResult(stdout.Bytes(), duration, artifacts.LogFile, artifacts.ResultFile)
	if result != nil {
		e.applyArtifactMetadata(result, artifacts, meta)
	}
	if runErr != nil {
		exitCode = 1
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		if result == nil {
			result = &Result{Output: strings.TrimSpace(stdout.String() + stderr.String()), Duration: duration}
			e.applyArtifactMetadata(result, artifacts, meta)
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
		fallback := &Result{
			Output:   strings.TrimSpace(stdout.String() + stderr.String()),
			ExitCode: exitCode,
			Duration: duration,
		}
		e.applyArtifactMetadata(fallback, artifacts, meta)
		return fallback, fmt.Errorf("解析 Claude JSON 输出失败: %w", parseErr)
	}
	result.ExitCode = exitCode
	return result, nil
}

type runMetadata struct {
	PromptFile      string `json:"prompt_file"`
	RTKEnabled      bool   `json:"rtk_enabled"`
	RTKMarkerFile   string `json:"rtk_marker_file,omitempty"`
	ResumeSessionID string `json:"resume_session_id,omitempty"`
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

func (e *Executor) prepareArtifacts(taskID string, opts RunOptions) (Result, runMetadata, error) {
	artifacts := e.ArtifactPaths(taskID)
	promptFile, err := e.archivePrompt(taskID, opts)
	if err != nil {
		return Result{}, runMetadata{}, err
	}
	meta := runMetadata{
		PromptFile:      promptFile,
		RTKEnabled:      e.inferRTKEnabled(),
		RTKMarkerFile:   filepath.Join(filepath.Dir(artifacts.MetaFile), safeTaskID(taskID)+".rtk"),
		ResumeSessionID: opts.ResumeSessionID,
	}
	e.applyArtifactMetadata(&artifacts, artifacts, meta)
	return artifacts, meta, nil
}

func (e *Executor) archivePrompt(taskID string, opts RunOptions) (string, error) {
	if err := os.MkdirAll(e.promptDir, 0o755); err != nil {
		return "", fmt.Errorf("创建 prompt 归档目录失败: %w", err)
	}
	ts := time.Now().Format("20060102_150405")
	path := filepath.Join(e.promptDir, fmt.Sprintf("%s_%s.md", safeTaskID(taskID), ts))
	var sb strings.Builder
	sb.WriteString("# ccclaw prompt archive\n\n")
	sb.WriteString(fmt.Sprintf("- task_id: %s\n", taskID))
	sb.WriteString(fmt.Sprintf("- archived_at: %s\n", time.Now().Format(time.RFC3339)))
	if strings.TrimSpace(opts.ResumeSessionID) != "" {
		sb.WriteString(fmt.Sprintf("- resume_session_id: %s\n", opts.ResumeSessionID))
	}
	sb.WriteString("\n---\n\n")
	sb.WriteString(opts.Prompt)
	sb.WriteString("\n")
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return "", fmt.Errorf("写入 prompt 归档失败: %w", err)
	}
	return path, nil
}

func (e *Executor) writeRunMetadata(path string, meta runMetadata) error {
	payload, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化执行元数据失败: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("写入执行元数据失败: %w", err)
	}
	return nil
}

func (e *Executor) loadRunMetadata(path string) (runMetadata, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return runMetadata{}, nil
		}
		return runMetadata{}, fmt.Errorf("读取执行元数据失败: %w", err)
	}
	var meta runMetadata
	if err := json.Unmarshal(payload, &meta); err != nil {
		return runMetadata{}, fmt.Errorf("解析执行元数据失败: %w", err)
	}
	return meta, nil
}

func (e *Executor) applyArtifactMetadata(result *Result, artifacts Result, meta runMetadata) {
	if result == nil {
		return
	}
	result.SessionName = artifacts.SessionName
	result.LogFile = artifacts.LogFile
	result.ResultFile = artifacts.ResultFile
	result.MetaFile = artifacts.MetaFile
	result.PromptFile = meta.PromptFile
	result.RTKEnabled = e.resolveRTKEnabled(meta)
	result.ResumeSessionID = meta.ResumeSessionID
}

func (e *Executor) commandArgs(opts RunOptions) []string {
	args := append([]string{}, e.command[1:]...)
	if strings.TrimSpace(opts.ResumeSessionID) != "" {
		args = append(args, "--resume", opts.ResumeSessionID)
	}
	args = append(args,
		"-p", opts.Prompt,
		"--dangerously-skip-permissions",
		"--output-format", "json",
	)
	return args
}

func (e *Executor) commandWithEnv(opts RunOptions, rtkMarkerFile string) []string {
	commandLine := append([]string{}, e.command...)
	envMap := e.runtimeEnv(rtkMarkerFile)
	if len(envMap) == 0 {
		return append(commandLine, e.commandArgs(opts)...)
	}
	envArgs := []string{"env"}
	keys := make([]string, 0, len(envMap))
	for key := range envMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		envArgs = append(envArgs, key+"="+envMap[key])
	}
	envArgs = append(envArgs, commandLine...)
	envArgs = append(envArgs, e.commandArgs(opts)...)
	return envArgs
}

func (e *Executor) runtimeEnv(rtkMarkerFile string) map[string]string {
	envMap := copyMap(e.secrets)
	if strings.TrimSpace(rtkMarkerFile) != "" {
		envMap["CCCLAW_RTK_MARKER_FILE"] = rtkMarkerFile
	}
	return envMap
}

func (e *Executor) resolveRTKEnabled(meta runMetadata) bool {
	if enabled, found := readRTKMarker(meta.RTKMarkerFile); found {
		return enabled
	}
	if meta.RTKEnabled {
		return true
	}
	return e.inferRTKEnabled()
}

func (e *Executor) inferRTKEnabled() bool {
	for _, part := range e.command {
		base := strings.ToLower(filepath.Base(strings.TrimSpace(part)))
		if base == "rtk" {
			return true
		}
		if strings.Contains(base, "ccclaude") {
			return tmux.Available("rtk")
		}
	}
	return false
}

func readRTKMarker(path string) (bool, bool) {
	if strings.TrimSpace(path) == "" {
		return false, false
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return false, false
	}
	switch strings.TrimSpace(string(payload)) {
	case "1", "true", "rtk":
		return true, true
	case "0", "false", "plain":
		return false, true
	default:
		return false, false
	}
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
