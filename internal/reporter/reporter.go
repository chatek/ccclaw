// Package reporter 回写执行结果到 GitHub Issue
package reporter

import (
	"fmt"
	"strings"
	"time"

	"github.com/NOTAschool/ccclaw/internal/gh"
	"github.com/NOTAschool/ccclaw/internal/task"
)

// Reporter 结果回写器
type Reporter struct {
	gh *gh.Client
}

// New 创建 Reporter
func New(client *gh.Client) *Reporter {
	return &Reporter{gh: client}
}

// ReportSuccess 回写成功结果
func (r *Reporter) ReportSuccess(t *task.Task, output string, duration time.Duration) error {
	body := fmt.Sprintf("## ccclaw 执行完成 ✓\n\n"+
		"**task_id**: `%s`\n"+
		"**耗时**: %s\n"+
		"**状态**: DONE\n\n"+
		"### 执行输出\n\n```\n%s\n```\n",
		t.TaskID, duration.Round(time.Second), truncate(output, 3000))
	return r.gh.AddComment(t.IssueNumber, body)
}

// ReportFailure 回写失败结果
func (r *Reporter) ReportFailure(t *task.Task, errMsg string) error {
	body := fmt.Sprintf("## ccclaw 执行失败 ✗\n\n"+
		"**task_id**: `%s`\n"+
		"**重试次数**: %d/3\n"+
		"**错误**: %s\n",
		t.TaskID, t.RetryCount, errMsg)
	if t.RetryCount >= 3 {
		body += "\n> 已达最大重试次数，任务进入死信队列，请人工介入。"
	}
	return r.gh.AddComment(t.IssueNumber, body)
}

// ReportBlocked 回写需要审批
func (r *Reporter) ReportBlocked(t *task.Task) error {
	body := fmt.Sprintf("## ccclaw 等待审批\n\n"+
		"**task_id**: `%s`\n"+
		"**风险等级**: `%s`\n\n"+
		"此任务为 high-risk，需要添加 `approved` 标签后方可执行。",
		t.TaskID, t.RiskLevel)
	return r.gh.AddComment(t.IssueNumber, body)
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...(输出已截断)"
}
