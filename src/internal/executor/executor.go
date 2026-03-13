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
	DiagnosticFile  string
	StdoutFile      string
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
	claudeBin   string
	rtkBin      string
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
	launcher, err := ResolveBinaryPath(resolved[0])
	if err != nil {
		return nil, fmt.Errorf("找不到执行器命令 %q: %w", resolved[0], err)
	}
	resolved[0] = launcher
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
		claudeBin:   configuredOrDiscoveredBinary(secrets, "CCCLAW_CLAUDE_BIN", "claude"),
		rtkBin:      configuredOrDiscoveredBinary(secrets, "CCCLAW_RTK_BIN", "rtk"),
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
		SessionName:    sessionName(taskID),
		LogFile:        filepath.Join(e.logDir, safe+".log"),
		ResultFile:     filepath.Join(e.resultDir, safe+".json"),
		DiagnosticFile: filepath.Join(e.resultDir, safe+".diag.txt"),
		StdoutFile:     filepath.Join(e.resultDir, safe+".stdout.txt"),
		MetaFile:       filepath.Join(e.resultDir, safe+".meta.json"),
	}
}

func (e *Executor) LoadResult(taskID string) (*Result, error) {
	artifacts := e.ArtifactPaths(taskID)
	meta, metaErr := e.loadRunMetadata(artifacts.MetaFile)
	if metaErr != nil {
		return nil, metaErr
	}
	resultPayload, err := os.ReadFile(artifacts.ResultFile)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("读取结构化结果文件失败: %w", err)
	}
	diagnosticPayload, diagnosticErr := os.ReadFile(artifacts.DiagnosticFile)
	if diagnosticErr != nil && !os.IsNotExist(diagnosticErr) {
		return nil, fmt.Errorf("读取诊断文件失败: %w", diagnosticErr)
	}
	stdoutPayload, stdoutErr := os.ReadFile(artifacts.StdoutFile)
	if stdoutErr != nil && !os.IsNotExist(stdoutErr) {
		return nil, fmt.Errorf("读取 stdout 暂存文件失败: %w", stdoutErr)
	}
	resultPayload, diagnosticPayload, materializeErr := e.materializeTMuxStdoutArtifact(artifacts, resultPayload, diagnosticPayload, stdoutPayload)
	if materializeErr != nil {
		return nil, materializeErr
	}
	result, parseErr := parseClaudeResult(resultPayload, 0, artifacts.LogFile, artifacts.ResultFile)
	if result != nil {
		e.applyArtifactMetadata(result, artifacts, meta)
	}
	if parseErr == nil {
		return result, nil
	}
	if hasStructuredClaudePayload(resultPayload) {
		if result != nil {
			return result, parseErr
		}
		return nil, parseErr
	}
	diagnosticPayload, sanitizeErr := e.normalizeDiagnosticArtifacts(artifacts, resultPayload, diagnosticPayload, parseErr)
	if sanitizeErr != nil {
		return nil, sanitizeErr
	}
	fallback := fallbackResultFromRaw(diagnosticPayload, artifacts.LogFile, artifacts.ResultFile, artifacts.DiagnosticFile)
	if fallback != nil {
		e.applyArtifactMetadata(fallback, artifacts, meta)
		return fallback, wrapStructuredResultError(resultPayload, parseErr, true)
	}
	if result != nil {
		return result, wrapStructuredResultError(resultPayload, parseErr, false)
	}
	return nil, wrapStructuredResultError(resultPayload, parseErr, false)
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
	if err := os.Remove(artifacts.DiagnosticFile); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("清理旧诊断失败: %w", err)
	}
	if err := os.Remove(artifacts.StdoutFile); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("清理旧 stdout 暂存失败: %w", err)
	}
	if err := os.Remove(artifacts.MetaFile); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("清理旧元数据失败: %w", err)
	}
	if err := e.initializeArtifactFiles(artifacts); err != nil {
		return nil, err
	}
	if err := e.writeRunMetadata(artifacts.MetaFile, meta); err != nil {
		return nil, err
	}
	launchCommand := e.buildTMuxCommand(taskID, opts, artifacts.StdoutFile, artifacts.DiagnosticFile, artifacts.LogFile)
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

func (e *Executor) buildTMuxCommand(taskID string, opts RunOptions, stdoutFile, diagnosticFile, logFile string) string {
	commandLine := shellJoin(e.commandWithEnv(taskID, opts, ""))
	script := fmt.Sprintf(
		"set -o pipefail; %s 1> >(tee %s >> %s) 2> >(tee %s >> %s >&2)",
		commandLine,
		shellQuote(stdoutFile),
		shellQuote(logFile),
		shellQuote(diagnosticFile),
		shellQuote(logFile),
	)
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
	if err := os.Remove(artifacts.DiagnosticFile); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("清理旧诊断失败: %w", err)
	}
	if err := os.Remove(artifacts.StdoutFile); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("清理旧 stdout 暂存失败: %w", err)
	}
	if err := os.Remove(artifacts.MetaFile); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("清理旧元数据失败: %w", err)
	}
	if err := e.initializeArtifactFiles(artifacts); err != nil {
		return nil, err
	}
	if err := e.writeRunMetadata(artifacts.MetaFile, meta); err != nil {
		return nil, err
	}

	logHandle, err := os.Create(artifacts.LogFile)
	if err != nil {
		return nil, fmt.Errorf("创建日志文件失败: %w", err)
	}
	defer logHandle.Close()

	diagnosticHandle, err := os.OpenFile(artifacts.DiagnosticFile, os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("打开诊断文件失败: %w", err)
	}
	defer diagnosticHandle.Close()

	args := e.commandArgs(opts)
	cmd := exec.CommandContext(ctx, e.command[0], args...)
	cmd.Dir = repoPath
	cmd.Env = append(cmd.Environ(), mapToEnv(e.runtimeEnv(taskID, meta.RTKMarkerFile))...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = io.MultiWriter(logHandle, &stdout)
	cmd.Stderr = io.MultiWriter(logHandle, diagnosticHandle, &stderr)

	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start)
	exitCode := 0
	result, parseErr := parseClaudeResult(stdout.Bytes(), duration, artifacts.LogFile, artifacts.ResultFile)
	if persistErr := e.persistStructuredArtifacts(artifacts, stdout.Bytes(), stderr.Bytes(), parseErr); persistErr != nil {
		return nil, persistErr
	}
	if result != nil {
		e.applyArtifactMetadata(result, artifacts, meta)
	}
	if runErr != nil {
		exitCode = 1
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		if result == nil {
			result = &Result{Output: strings.TrimSpace(string(joinDiagnosticPayloads(stdout.Bytes(), stderr.Bytes()))), Duration: duration}
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
			Output:   strings.TrimSpace(string(joinDiagnosticPayloads(stdout.Bytes(), stderr.Bytes()))),
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
		RTKEnabled:      false,
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
	result.DiagnosticFile = artifacts.DiagnosticFile
	result.StdoutFile = artifacts.StdoutFile
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

func (e *Executor) commandWithEnv(taskID string, opts RunOptions, rtkMarkerFile string) []string {
	commandLine := append([]string{}, e.command...)
	envMap := e.runtimeEnv(taskID, rtkMarkerFile)
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

func (e *Executor) runtimeEnv(taskID, _ string) map[string]string {
	envMap := copyMap(e.secrets)
	if strings.TrimSpace(envMap["CCCLAW_CLAUDE_BIN"]) == "" && strings.TrimSpace(e.claudeBin) != "" {
		envMap["CCCLAW_CLAUDE_BIN"] = e.claudeBin
	}
	appDir := filepath.Dir(filepath.Dir(e.resultDir))
	if strings.TrimSpace(appDir) != "" {
		envMap["CCCLAW_APP_DIR"] = appDir
		envMap["CCCLAW_HOOK_STATE_DIR"] = filepath.Join(filepath.Dir(e.resultDir), "claude-hooks")
	}
	if strings.TrimSpace(taskID) != "" {
		envMap["CCCLAW_TASK_ID"] = taskID
	}
	return envMap
}

func (e *Executor) resolveRTKEnabled(meta runMetadata) bool {
	return false
}

func (e *Executor) inferRTKEnabled() bool {
	return false
}

func fallbackResultFromRaw(raw []byte, logFile, resultFile, diagnosticFile string) *Result {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil
	}
	return &Result{
		Output:         text,
		LogFile:        logFile,
		ResultFile:     resultFile,
		DiagnosticFile: diagnosticFile,
	}
}

func (e *Executor) initializeArtifactFiles(artifacts Result) error {
	for _, item := range []struct {
		path string
		name string
	}{
		{path: artifacts.ResultFile, name: "结构化结果文件"},
		{path: artifacts.DiagnosticFile, name: "诊断文件"},
		{path: artifacts.StdoutFile, name: "stdout 暂存文件"},
	} {
		if err := os.WriteFile(item.path, nil, 0o644); err != nil {
			return fmt.Errorf("初始化%s失败: %w", item.name, err)
		}
	}
	return nil
}

func (e *Executor) persistStructuredArtifacts(artifacts Result, stdoutPayload, stderrPayload []byte, parseErr error) error {
	if parseErr == nil || hasStructuredClaudePayload(stdoutPayload) {
		if err := os.WriteFile(artifacts.ResultFile, stdoutPayload, 0o644); err != nil {
			return fmt.Errorf("写入结构化结果文件失败: %w", err)
		}
		if err := os.WriteFile(artifacts.DiagnosticFile, stderrPayload, 0o644); err != nil {
			return fmt.Errorf("写入诊断文件失败: %w", err)
		}
		return nil
	}
	if err := os.WriteFile(artifacts.ResultFile, nil, 0o644); err != nil {
		return fmt.Errorf("清空结构化结果文件失败: %w", err)
	}
	if err := os.WriteFile(artifacts.DiagnosticFile, joinDiagnosticPayloads(stdoutPayload, stderrPayload), 0o644); err != nil {
		return fmt.Errorf("写入诊断文件失败: %w", err)
	}
	return nil
}

func (e *Executor) normalizeDiagnosticArtifacts(artifacts Result, resultPayload, diagnosticPayload []byte, parseErr error) ([]byte, error) {
	trimmedResult := bytes.TrimSpace(resultPayload)
	trimmedDiagnostic := bytes.TrimSpace(diagnosticPayload)
	if len(trimmedResult) == 0 {
		return diagnosticPayload, nil
	}
	if parseErr == nil {
		return diagnosticPayload, nil
	}
	if hasStructuredClaudePayload(resultPayload) {
		return diagnosticPayload, nil
	}
	merged := joinDiagnosticPayloads(resultPayload, diagnosticPayload)
	if !bytes.Equal(bytes.TrimSpace(merged), trimmedDiagnostic) {
		if err := os.WriteFile(artifacts.DiagnosticFile, merged, 0o644); err != nil {
			return nil, fmt.Errorf("回写诊断文件失败: %w", err)
		}
	}
	if err := os.WriteFile(artifacts.ResultFile, nil, 0o644); err != nil {
		return nil, fmt.Errorf("清空无效结构化结果文件失败: %w", err)
	}
	return merged, nil
}

func (e *Executor) materializeTMuxStdoutArtifact(artifacts Result, resultPayload, diagnosticPayload, stdoutPayload []byte) ([]byte, []byte, error) {
	trimmedStdout := bytes.TrimSpace(stdoutPayload)
	if len(trimmedStdout) == 0 {
		return resultPayload, diagnosticPayload, nil
	}
	trimmedResult := bytes.TrimSpace(resultPayload)
	if len(trimmedResult) > 0 {
		if err := removeIfExists(artifacts.StdoutFile); err != nil {
			return nil, nil, fmt.Errorf("清理已消费 stdout 暂存文件失败: %w", err)
		}
		return resultPayload, diagnosticPayload, nil
	}
	if hasStructuredClaudePayload(stdoutPayload) {
		if err := os.WriteFile(artifacts.ResultFile, stdoutPayload, 0o644); err != nil {
			return nil, nil, fmt.Errorf("提升 stdout 暂存为结构化结果失败: %w", err)
		}
		if err := removeIfExists(artifacts.StdoutFile); err != nil {
			return nil, nil, fmt.Errorf("清理已提升 stdout 暂存文件失败: %w", err)
		}
		return stdoutPayload, diagnosticPayload, nil
	}
	merged := joinDiagnosticPayloads(stdoutPayload, diagnosticPayload)
	if err := os.WriteFile(artifacts.DiagnosticFile, merged, 0o644); err != nil {
		return nil, nil, fmt.Errorf("迁移 stdout 暂存到诊断文件失败: %w", err)
	}
	if err := os.WriteFile(artifacts.ResultFile, nil, 0o644); err != nil {
		return nil, nil, fmt.Errorf("清空结构化结果文件失败: %w", err)
	}
	if err := removeIfExists(artifacts.StdoutFile); err != nil {
		return nil, nil, fmt.Errorf("清理无效 stdout 暂存文件失败: %w", err)
	}
	return nil, merged, nil
}

func wrapStructuredResultError(resultPayload []byte, parseErr error, hasDiagnostic bool) error {
	trimmed := bytes.TrimSpace(resultPayload)
	if len(trimmed) == 0 {
		if hasDiagnostic {
			return fmt.Errorf("结构化结果缺失，已回退诊断文件")
		}
		return fmt.Errorf("结构化结果缺失")
	}
	if parseErr == nil {
		if hasDiagnostic {
			return fmt.Errorf("结构化结果缺失，已回退诊断文件")
		}
		return fmt.Errorf("结构化结果缺失")
	}
	if hasDiagnostic {
		return fmt.Errorf("结构化结果解析失败: %w；已回退诊断文件", parseErr)
	}
	return fmt.Errorf("结构化结果解析失败: %w", parseErr)
}

func joinDiagnosticPayloads(chunks ...[]byte) []byte {
	parts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		text := strings.TrimSpace(string(chunk))
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	if len(parts) == 0 {
		return nil
	}
	return []byte(strings.Join(parts, "\n"))
}

func hasStructuredClaudePayload(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false
	}
	var payload claudeJSONResult
	return json.Unmarshal(trimmed, &payload) == nil
}

func removeIfExists(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func configuredOrDiscoveredBinary(secrets map[string]string, envKey, name string) string {
	if override := strings.TrimSpace(expandUserPath(secrets[envKey])); override != "" {
		return override
	}
	path, err := ResolveBinaryPath(name)
	if err != nil {
		return ""
	}
	return path
}

func ResolveBinaryPath(name string) (string, error) {
	trimmed := strings.TrimSpace(expandUserPath(name))
	if trimmed == "" {
		return "", fmt.Errorf("命令名为空")
	}
	if strings.Contains(trimmed, string(os.PathSeparator)) {
		if isExecutableFile(trimmed) {
			return trimmed, nil
		}
		return "", exec.ErrNotFound
	}
	if path, err := exec.LookPath(trimmed); err == nil {
		return path, nil
	}
	for _, dir := range candidateBinaryDirs() {
		candidate := filepath.Join(dir, trimmed)
		if isExecutableFile(candidate) {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}

func candidateBinaryDirs() []string {
	seen := map[string]struct{}{}
	dirs := make([]string, 0, 8)
	appendDir := func(path string) {
		path = strings.TrimSpace(expandUserPath(path))
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		dirs = append(dirs, path)
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		appendDir(dir)
	}
	if home, err := os.UserHomeDir(); err == nil {
		appendDir(filepath.Join(home, ".local", "bin"))
		appendDir(filepath.Join(home, "bin"))
	}
	return dirs
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}

func expandUserPath(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if len(path) > 1 && path[1] == os.PathSeparator {
		return filepath.Join(home, path[2:])
	}
	return path
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
