package reporter

import (
	"fmt"
	"strings"
	"time"

	"github.com/41490/ccclaw/internal/adapters/github"
	"github.com/41490/ccclaw/internal/core"
)

type Reporter struct {
	clientForRepo func(string) *github.Client
}

func New(clientForRepo func(string) *github.Client) *Reporter {
	return &Reporter{clientForRepo: clientForRepo}
}

func (r *Reporter) client(task *core.Task) *github.Client {
	if r == nil || r.clientForRepo == nil {
		return nil
	}
	repo := strings.TrimSpace(task.IssueRepo)
	if repo == "" {
		repo = strings.TrimSpace(task.ControlRepo)
	}
	return r.clientForRepo(repo)
}

func (r *Reporter) ReportBlocked(task *core.Task, reason string) error {
	client := r.client(task)
	if client == nil {
		return nil
	}
	body := fmt.Sprintf("任务已进入阻塞状态。\n\n- Issue: %s#%d\n- 原因: %s\n- 当前状态: `%s`", task.IssueRepo, task.IssueNumber, reason, task.State)
	return client.AddComment(task.IssueNumber, body)
}

func (r *Reporter) ReportFailure(task *core.Task) error {
	client := r.client(task)
	if client == nil {
		return nil
	}
	body := fmt.Sprintf("任务执行失败。\n\n- Issue: %s#%d\n- 状态: `%s`\n- 重试次数: `%d/%d`\n- 错误: `%s`", task.IssueRepo, task.IssueNumber, task.State, task.RetryCount, core.MaxRetry, strings.TrimSpace(task.ErrorMsg))
	return client.AddComment(task.IssueNumber, body)
}

func (r *Reporter) ReportSuccess(task *core.Task, duration time.Duration, logFile string) error {
	client := r.client(task)
	if client == nil {
		return nil
	}
	body := fmt.Sprintf("任务执行完成。\n\n- Issue: %s#%d\n- 状态: `%s`\n- 耗时: `%s`\n- 工程报告: `%s`\n- 日志: `%s`", task.IssueRepo, task.IssueNumber, task.State, duration.Round(time.Second), task.ReportPath, logFile)
	return client.AddComment(task.IssueNumber, body)
}
