// Package executor 调用 Claude Code worker 执行任务
package executor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Minute

// Result 执行结果
type Result struct {
	Output   string
	ExitCode int
	Duration time.Duration
	WorkDir  string
}

// Executor Claude Code 执行器
type Executor struct {
	claudeBin    string
	logDir       string
	workspaceDir string
}

// New 创建执行器
func New(claudeBin, logDir, workspaceDir string) (*Executor, error) {
	if claudeBin == "" {
		claudeBin = "claude"
	}
	if _, err := exec.LookPath(claudeBin); err != nil {
		return nil, fmt.Errorf("找不到 claude 可执行文件 %q: %w", claudeBin, err)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建工作区目录失败: %w", err)
	}
	return &Executor{claudeBin: claudeBin, logDir: logDir, workspaceDir: workspaceDir}, nil
}

// Run 执行任务，在独立工作目录中以 -p agent 模式运行 claude
func (e *Executor) Run(taskID, prompt string) (*Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	// 创建任务专属工作目录
	safe := strings.NewReplacer("#", "_", "/", "_").Replace(taskID)
	workDir := filepath.Join(e.workspaceDir, safe)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建工作目录失败: %w", err)
	}

	// 将 prompt 写入 TASK.md，供 claude 读取
	taskFile := filepath.Join(workDir, "TASK.md")
	if err := os.WriteFile(taskFile, []byte(prompt), 0o644); err != nil {
		return nil, fmt.Errorf("写入任务文件失败: %w", err)
	}

	logFile := filepath.Join(e.logDir, fmt.Sprintf("%s_%d.log", safe, time.Now().Unix()))
	slog.Info("开始执行任务", "task_id", taskID, "work_dir", workDir, "log", logFile)

	start := time.Now()

	// -p 模式：agent 化，有工具调用权限（读写文件、执行命令）
	// --dangerously-skip-permissions：在 systemd ReadWritePaths 沙箱内运行
	cmd := exec.CommandContext(ctx, e.claudeBin,
		"-p", prompt,
		"--dangerously-skip-permissions",
		"--output-format", "text",
	)
	cmd.Dir = workDir

	logF, err := os.Create(logFile)
	if err != nil {
		slog.Warn("创建日志文件失败", "err", err)
	} else {
		defer logF.Close()
		cmd.Stdout = logF
		cmd.Stderr = logF
	}

	runErr := cmd.Run()
	duration := time.Since(start)

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

	// 读取日志作为输出摘要
	output := ""
	if logF != nil {
		if data, readErr := os.ReadFile(logFile); readErr == nil {
			output = string(data)
		}
	}

	return &Result{
		Output:   output,
		ExitCode: exitCode,
		Duration: duration,
		WorkDir:  workDir,
	}, nil
}
