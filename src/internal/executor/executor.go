package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Result struct {
	Output   string
	ExitCode int
	Duration time.Duration
	LogFile  string
}

type Executor struct {
	binary  string
	timeout time.Duration
	logDir  string
	secrets map[string]string
}

func New(binary string, timeout time.Duration, logDir string, secrets map[string]string) (*Executor, error) {
	if binary == "" {
		binary = "claude"
	}
	if _, err := exec.LookPath(binary); err != nil {
		return nil, fmt.Errorf("找不到执行器二进制 %q: %w", binary, err)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	return &Executor{binary: binary, timeout: timeout, logDir: logDir, secrets: copyMap(secrets)}, nil
}

func (e *Executor) Run(parent context.Context, repoPath, taskID, prompt string) (*Result, error) {
	ctx, cancel := context.WithTimeout(parent, e.timeout)
	defer cancel()

	safe := strings.NewReplacer("#", "_", "/", "_").Replace(taskID)
	logFile := filepath.Join(e.logDir, fmt.Sprintf("%s_%d.log", safe, time.Now().Unix()))
	logHandle, err := os.Create(logFile)
	if err != nil {
		return nil, fmt.Errorf("创建日志文件失败: %w", err)
	}
	defer logHandle.Close()

	cmd := exec.CommandContext(ctx, e.binary,
		"-p", prompt,
		"--dangerously-skip-permissions",
		"--output-format", "text",
	)
	cmd.Dir = repoPath
	cmd.Env = append(cmd.Environ(), mapToEnv(e.secrets)...)

	var output bytes.Buffer
	writer := io.MultiWriter(logHandle, &output)
	cmd.Stdout = writer
	cmd.Stderr = writer

	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start)
	exitCode := 0
	if runErr != nil {
		exitCode = 1
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return &Result{Output: output.String(), ExitCode: exitCode, Duration: duration, LogFile: logFile}, fmt.Errorf("执行器运行失败: %w", runErr)
	}
	return &Result{Output: output.String(), ExitCode: exitCode, Duration: duration, LogFile: logFile}, nil
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
