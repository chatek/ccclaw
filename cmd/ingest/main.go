// ccclaw ingest — 拉取 GitHub Issues 并写入任务状态
package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/NOTAschool/ccclaw/internal/gh"
	"github.com/NOTAschool/ccclaw/internal/task"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	repo := mustEnv("CCCLAW_REPO")
	stateDir := envOr("CCCLAW_STATE_DIR", "/var/lib/ccclaw")
	label := envOr("CCCLAW_INGEST_LABEL", "ccclaw")
	limitStr := envOr("CCCLAW_INGEST_LIMIT", "20")
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		limit = 20
	}

	store, err := task.NewStore(stateDir)
	if err != nil {
		slog.Error("初始化状态存储失败", "err", err)
		os.Exit(1)
	}

	client := gh.NewClient(repo)
	issues, err := client.ListOpenIssues(label, limit)
	if err != nil {
		slog.Error("拉取 Issues 失败", "err", err)
		os.Exit(1)
	}

	ingested := 0
	skipped := 0
	for _, issue := range issues {
		ikey := fmt.Sprintf("%d#body", issue.Number)
		exists, err := store.Exists(ikey)
		if err != nil {
			slog.Warn("检查幂等键失败", "ikey", ikey, "err", err)
			continue
		}
		if exists {
			skipped++
			continue
		}

		// 判断风险等级
		risk := task.RiskLow
		if issue.HasLabel("risk:high") {
			risk = task.RiskHigh
		} else if issue.HasLabel("risk:med") {
			risk = task.RiskMed
		}

		// 判断是否已获审批
		approved := issue.HasLabel("approved")

		t := &task.Task{
			TaskID:         fmt.Sprintf("%d#body", issue.Number),
			IdempotencyKey: ikey,
			Intent:         inferIntent(issue.LabelNames()),
			RiskLevel:      risk,
			Approved:       approved,
			IssueNumber:      issue.Number,
			IssueTitle:       issue.Title,
			IssueBody:        issue.Body,
			Labels:           issue.LabelNames(),
			State:            task.StateNew,
			CreatedAt:        issue.CreatedAt,
		}

		if err := store.Save(t); err != nil {
			slog.Error("保存任务失败", "ikey", ikey, "err", err)
			continue
		}
		slog.Info("新任务已入队", "issue", issue.Number, "title", issue.Title, "risk", risk)
		ingested++
	}

	slog.Info("ingest 完成", "ingested", ingested, "skipped", skipped)
}

func inferIntent(labels []string) task.Intent {
	for _, l := range labels {
		switch l {
		case "ops":
			return task.IntentOps
		case "bug":
			return task.IntentFix
		case "release":
			return task.IntentRelease
		case "research":
			return task.IntentResearch
		}
	}
	return task.IntentResearch
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
