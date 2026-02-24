// Package gh 封装 GitHub API 调用（通过 gh CLI）
package gh

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Issue GitHub Issue 结构
type Issue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	Labels    []Label   `json:"labels"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Label GitHub 标签
type Label struct {
	Name string `json:"name"`
}

// LabelNames 返回标签名列表
func (i *Issue) LabelNames() []string {
	names := make([]string, len(i.Labels))
	for idx, l := range i.Labels {
		names[idx] = l.Name
	}
	return names
}

// HasLabel 检查是否含有指定标签
func (i *Issue) HasLabel(label string) bool {
	for _, l := range i.Labels {
		if l.Name == label {
			return true
		}
	}
	return false
}

// Client GitHub API 客户端（基于 gh CLI）
type Client struct {
	repo string // owner/repo
}

// NewClient 创建客户端
func NewClient(repo string) *Client {
	return &Client{repo: repo}
}

// ListOpenIssues 拉取开放的 Issues（带指定标签）
func (c *Client) ListOpenIssues(label string, limit int) ([]Issue, error) {
	args := []string{
		"issue", "list",
		"--repo", c.repo,
		"--state", "open",
		"--json", "number,title,body,state,labels,createdAt,updatedAt",
		"--limit", strconv.Itoa(limit),
	}
	if label != "" {
		args = append(args, "--label", label)
	}

	out, err := runGH(args...)
	if err != nil {
		return nil, fmt.Errorf("gh issue list 失败: %w", err)
	}

	var issues []Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("解析 issue 列表失败: %w", err)
	}
	return issues, nil
}

// GetIssue 获取单个 Issue
func (c *Client) GetIssue(number int) (*Issue, error) {
	out, err := runGH(
		"issue", "view", strconv.Itoa(number),
		"--repo", c.repo,
		"--json", "number,title,body,state,labels,createdAt,updatedAt",
	)
	if err != nil {
		return nil, fmt.Errorf("gh issue view %d 失败: %w", number, err)
	}

	var issue Issue
	if err := json.Unmarshal(out, &issue); err != nil {
		return nil, fmt.Errorf("解析 issue 失败: %w", err)
	}
	return &issue, nil
}

// AddComment 在 Issue 上添加评论
func (c *Client) AddComment(number int, body string) error {
	_, err := runGH(
		"issue", "comment", strconv.Itoa(number),
		"--repo", c.repo,
		"--body", body,
	)
	if err != nil {
		return fmt.Errorf("gh issue comment %d 失败: %w", number, err)
	}
	return nil
}

// AddLabel 给 Issue 添加标签
func (c *Client) AddLabel(number int, label string) error {
	_, err := runGH(
		"issue", "edit", strconv.Itoa(number),
		"--repo", c.repo,
		"--add-label", label,
	)
	if err != nil {
		return fmt.Errorf("gh issue edit %d --add-label %s 失败: %w", number, label, err)
	}
	return nil
}

// runGH 执行 gh 命令，返回 stdout
func runGH(args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("gh %s: %w (stderr: %s)", strings.Join(args[:2], " "), err, stderr)
	}
	return out, nil
}
