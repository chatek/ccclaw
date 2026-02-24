// ccclaw run — 执行队列中的 NEW/TRIAGED 任务（串行）
package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NOTAschool/ccclaw/internal/executor"
	"github.com/NOTAschool/ccclaw/internal/gh"
	"github.com/NOTAschool/ccclaw/internal/reporter"
	"github.com/NOTAschool/ccclaw/internal/skill"
	"github.com/NOTAschool/ccclaw/internal/task"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	repo := mustEnv("CCCLAW_REPO")
	stateDir := envOr("CCCLAW_STATE_DIR", "/var/lib/ccclaw")
	logDir := envOr("CCCLAW_LOG_DIR", "/var/log/ccclaw")
	claudeBin := envOr("CCCLAW_CLAUDE_BIN", "claude")
	workspaceDir := envOr("CCCLAW_WORKSPACE_DIR", "/var/lib/ccclaw/workspace")
	repoRoot := envOr("CCCLAW_REPO_ROOT", ".")
	skillsDir := envOr("CCCLAW_SKILLS_DIR", filepath.Join(repoRoot, "skills"))
	docsDir := envOr("CCCLAW_DOCS_DIR", filepath.Join(repoRoot, "docs"))

	store, err := task.NewStore(stateDir)
	if err != nil {
		slog.Error("初始化状态存储失败", "err", err)
		os.Exit(1)
	}

	exec, err := executor.New(claudeBin, logDir, workspaceDir)
	if err != nil {
		slog.Error("初始化执行器失败", "err", err)
		os.Exit(1)
	}

	skillIdx := skill.NewIndex(skillsDir)
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

		prompt, err := buildPrompt(t, skillIdx, docsDir)
		if err != nil {
			slog.Warn("构建 prompt 失败，使用基础 prompt", "err", err)
			prompt = buildBasePrompt(t)
		}
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

// extractKeywords 从任务标题和标签中提取关键词
func extractKeywords(t *task.Task) []string {
	kws := []string{string(t.Intent)}
	kws = append(kws, t.Labels...)
	// 标题按空格分词，取长度 > 2 的词
	for _, w := range strings.Fields(t.IssueTitle) {
		if len(w) > 2 {
			kws = append(kws, w)
		}
	}
	return kws
}

// buildPrompt 组装完整 prompt：任务 + docs 记忆 + skill + 自学习指令
func buildPrompt(t *task.Task, skillIdx *skill.Index, docsDir string) (string, error) {
	keywords := extractKeywords(t)

	var sb strings.Builder

	// 基础任务描述
	sb.WriteString(buildBasePrompt(t))

	// 注入匹配的 docs/ 记忆（Q3=B：关键词匹配）
	docs, err := skill.MatchDocs(docsDir, keywords)
	if err == nil && len(docs) > 0 {
		sb.WriteString("\n\n## 相关文档记忆（来自 docs/）\n")
		for _, d := range docs {
			sb.WriteString(fmt.Sprintf("\n### docs/%s/%s\n\n%s\n", d.SubDir, d.Name, d.Content))
		}
	}

	// 注入匹配的 skill
	skills, err := skillIdx.Match(keywords)
	if err == nil && len(skills) > 0 {
		sb.WriteString("\n\n## 相关 SKILL 记忆\n")
		for _, s := range skills {
			sb.WriteString(fmt.Sprintf("\n### [%s] %s\n\n%s\n", s.Level, s.Name, s.Content))
		}
	}

	// SKILL 自学习指令（Q4=A：自动写入，无需人工审核）
	sb.WriteString(`

## 执行后必须完成的自学习任务

任务执行完毕后，请将本次执行中发现的关键规律、SOP 或决策模式写入 skill 文件：

1. 如果发现可复用的操作规程（SOP），写入 skills/L1/<主题>.md
2. 如果发现需要条件判断的决策树，写入 skills/L2/<主题>.md
3. 将执行报告写入 docs/reports/<日期>_<issue号>_<主题>.md

skill 文件格式：
` + "```" + `markdown
# <技能名称>

## 适用场景
<描述何时使用此 skill>

## 步骤
1. ...
2. ...

## 注意事项
- ...
` + "```" + `

如果本次任务没有产生新的可复用规律，跳过 skill 写入，但仍需写执行报告。
`)

	return sb.String(), nil
}

// buildBasePrompt 基础任务 prompt（无记忆注入）
func buildBasePrompt(t *task.Task) string {
	return fmt.Sprintf(`你是 ccclaw 执行器，在项目仓库中自主运行。请处理以下 GitHub Issue 任务。

## Issue #%d: %s

%s

## 任务元数据
- 意图: %s
- 风险等级: %s
- 标签: %s

## 要求
- 分析任务意图，给出具体可执行的方案或结论
- 如需代码变更，直接修改文件（你有工具调用权限）
- 输出结构化的执行报告
`, t.IssueNumber, t.IssueTitle, t.IssueBody, t.Intent, t.RiskLevel, strings.Join(t.Labels, ", "))
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
