package reporter

import (
	"fmt"
	"strings"
	"time"

	"github.com/41490/ccclaw/internal/adapters/github"
	"github.com/41490/ccclaw/internal/core"
)

type Reporter struct {
	gh *github.Client
}

func New(gh *github.Client) *Reporter {
	return &Reporter{gh: gh}
}

func (r *Reporter) ReportBlocked(task *core.Task, reason string) error {
	body := fmt.Sprintf("任务已进入阻塞状态。\n\n- Issue: #%d\n- 原因: %s\n- 当前状态: `%s`", task.IssueNumber, reason, task.State)
	return r.gh.AddComment(task.IssueNumber, body)
}

func (r *Reporter) ReportFailure(task *core.Task) error {
	body := fmt.Sprintf("任务执行失败。\n\n- Issue: #%d\n- 状态: `%s`\n- 重试次数: `%d/%d`\n- 错误: `%s`", task.IssueNumber, task.State, task.RetryCount, core.MaxRetry, strings.TrimSpace(task.ErrorMsg))
	return r.gh.AddComment(task.IssueNumber, body)
}

func (r *Reporter) ReportSuccess(task *core.Task, duration time.Duration, logFile string) error {
	body := fmt.Sprintf("任务执行完成。\n\n- Issue: #%d\n- 状态: `%s`\n- 耗时: `%s`\n- 工程报告: `%s`\n- 日志: `%s`", task.IssueNumber, task.State, duration.Round(time.Second), task.ReportPath, logFile)
	return r.gh.AddComment(task.IssueNumber, body)
}
