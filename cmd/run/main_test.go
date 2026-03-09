package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NOTAschool/ccclaw/internal/skill"
	"github.com/NOTAschool/ccclaw/internal/task"
)

func TestBuildPromptIncludesPhaseBSections(t *testing.T) {
	tk := &task.Task{
		TaskID:      "2#body",
		IssueNumber: 2,
		IssueTitle:  "phase b ops workflow",
		IssueBody:   "需要完善任务执行编排",
		Intent:      task.IntentOps,
		Labels:      []string{"ccclaw"},
		RiskLevel:   task.RiskLow,
	}

	docsDir := filepath.Join(t.TempDir(), "docs")
	for _, sub := range []string{"designs", "plans", "assay", "reports"} {
		if err := os.MkdirAll(filepath.Join(docsDir, sub), 0o755); err != nil {
			t.Fatalf("创建 docs 子目录失败: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(docsDir, "plans", "phase-b.md"), []byte("ops workflow plan"), 0o644); err != nil {
		t.Fatalf("写入 docs 文件失败: %v", err)
	}

	skillsDir := filepath.Join(t.TempDir(), "skills")
	if err := os.MkdirAll(filepath.Join(skillsDir, "L1"), 0o755); err != nil {
		t.Fatalf("创建 skills 目录失败: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(skillsDir, "L2"), 0o755); err != nil {
		t.Fatalf("创建 skills 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "L1", "ops.md"), []byte("# ops\nworkflow"), 0o644); err != nil {
		t.Fatalf("写入 skills 文件失败: %v", err)
	}

	prompt, err := buildPrompt(tk, skill.NewIndex(skillsDir), docsDir)
	if err != nil {
		t.Fatalf("buildPrompt 返回错误: %v", err)
	}

	for _, want := range []string{
		"TodoWrite 风格计划模板",
		"Task 子任务编排协议",
		"Explore：收集上下文与约束",
		"PLAN",
		"CODE",
		"## 相关文档记忆（来自 docs/）",
		"## 相关 SKILL 记忆",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt 未包含关键片段: %q", want)
		}
	}
}

