// Package executor 调用 Claude Code worker 执行任务
package executor

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultTimeout = 10 * time.Minute

// Result 执行结果
type Result struct {
	Output   string
	ExitCode int
	Duration time.Duration
}

// Executor Claude Code 执行器
type Executor struct {
	claudeBin string
	logDir    string
}

// New 创建执行器
func New(claudeBin, logDir string) (*Executor, error) {
	if claudeBin == "" {
		claudeBin = "claude"
	}
	if _, err := exec.LookPath(claudeBin); err != nil {
		return nil, fmt.Errorf("找不到 claude 可执行文件 %q: %w", claudeBin, err)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	return &Executor{claudeBin: claudeBin, logDir: logDir}, nil
}

// Run 执行任务，prompt 为任务指令，taskID 用于日志命名
func (e *Executor) Run(taskID, prompt string) (*Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	// 构建日志文件路径
	safe := strings.NewReplacer("#", "_", "/", "_").Replace(taskID)
	logFile := filepath.Join(e.logDir, fmt.Sprintf("%s_%d.log", safe, time.Now().Unix()))

	slog.Info("开始执行任务", "task_id", taskID, "log", logFile)

	start := time.Now()

	// claude --print 模式：非交互，输出到 stdout
	cmd := exec.CommandContext(ctx, e.claudeBin, "--print", "--output-format", "text", prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	duration := time.Since(start)

	// 写日志
	logContent := fmt.Sprintf("=== task_id: %s ===\n=== 耗时: %s ===\n\n--- STDOUT ---\n%s\n--- STDERR ---\n%s\n",
		taskID, duration, stdout.String(), stderr.String())
	if writeErr := os.WriteFile(logFile, []byte(logContent), 0o644); writeErr != nil {
		slog.Warn("写日志失败", "err", writeErr)
	}

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() != nil {
			return nil, fmt.Errorf("任务超时（%s）: %w", defaultTimeout, ctx.Err())
		} else {
			return nil, fmt.Errorf("执行失败: %w", runErr)
		}
	}

	return &Result{
		Output:   stdout.String(),
		ExitCode: exitCode,
		Duration: duration,
	}, nil
}
