// ccclaw run — 执行队列中的 NEW/TRIAGED 任务（串行）
package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/NOTAschool/ccclaw/internal/executor"
	"github.com/NOTAschool/ccclaw/internal/gh"
	"github.com/NOTAschool/ccclaw/internal/reporter"
	"github.com/NOTAschool/ccclaw/internal/task"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	repo := mustEnv("CCCLAW_REPO")
	stateDir := envOr("CCCLAW_STATE_DIR", "/var/lib/ccclaw")
	logDir := envOr("CCCLAW_LOG_DIR", "/var/log/ccclaw")
	claudeBin := envOr("CCCLAW_CLAUDE_BIN", "claude")

	store, err := task.NewStore(stateDir)
	if err != nil {
		slog.Error("初始化状态存储失败", "err", err)
		os.Exit(1)
	}

	exec, err := executor.New(claudeBin, logDir)
	if err != nil {
		slog.Error("初始化执行器失败", "err", err)
		os.Exit(1)
	}

	ghClient := gh.NewClient(repo)
	rep := reporter.New(ghClient)

	tasks, err := store.List()
	if err != nil {
		slog.Error("读取任务列表失败", "err", err)
		os.Exit(1)
	}

	ran := 0
	for _, t := range tasks {
		if t.State != task.StateNew && t.State != task.StateTriaged {
			continue
		}

		// 风险门禁
		if err := t.CanExecute(); err != nil {
			slog.Warn("任务被门禁阻止", "task_id", t.TaskID, "err", err)
			if t.RiskLevel == task.RiskHigh {
				t.State = task.StateBlocked
				_ = store.Save(t)
				_ = rep.ReportBlocked(t)
			}
			continue
		}

		// 标记运行中
		t.State = task.StateRunning
		t.UpdatedAt = time.Now()
		if err := store.Save(t); err != nil {
			slog.Error("更新任务状态失败", "task_id", t.TaskID, "err", err)
			continue
		}

		slog.Info("开始执行任务", "task_id", t.TaskID, "issue", t.IssueNumber)

		prompt := buildPrompt(t)
		result, execErr := exec.Run(t.TaskID, prompt)

		if execErr != nil || (result != nil && result.ExitCode != 0) {
			errMsg := ""
			if execErr != nil {
				errMsg = execErr.Error()
			} else {
				errMsg = fmt.Sprintf("claude 退出码 %d\n%s", result.ExitCode, result.Output)
			}

			t.RetryCount++
			t.ErrorMsg = errMsg
			if t.RetryCount >= 3 {
				t.State = task.StateDead
				slog.Error("任务进入死信队列", "task_id", t.TaskID, "retries", t.RetryCount)
			} else {
				t.State = task.StateFailed
				slog.Warn("任务执行失败，将重试", "task_id", t.TaskID, "retry", t.RetryCount)
			}
			_ = store.Save(t)
			_ = rep.ReportFailure(t, errMsg)
			continue
		}

		// 成功
		t.State = task.StateDone
		t.ErrorMsg = ""
		_ = store.Save(t)
		_ = rep.ReportSuccess(t, result.Output, result.Duration)
		slog.Info("任务完成", "task_id", t.TaskID, "duration", result.Duration)
		ran++
	}

	slog.Info("run 完成", "ran", ran)
}

func buildPrompt(t *task.Task) string {
	return fmt.Sprintf(`你是 ccclaw 执行器。请处理以下 GitHub Issue 任务。

## Issue #%d: %s

%s

## 要求
- 分析任务意图（%s）
- 给出具体可执行的方案或结论
- 如需代码变更，列出文件和改动摘要
- 输出结构化的执行报告
`, t.IssueNumber, t.IssueTitle, t.IssueBody, t.Intent)
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "缺少必要环境变量: %s\n", key)
		os.Exit(1)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
