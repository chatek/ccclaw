package sevolver

import (
	"strings"
	"testing"
	"time"

	ghadapter "github.com/41490/ccclaw/internal/adapters/github"
)

type fakeDeepAnalysisClient struct {
	openIssues   []ghadapter.Issue
	createdIssue *ghadapter.Issue
	createCalls  int
	lastTitle    string
	lastBody     string
	lastLabels   []string
}

func (f *fakeDeepAnalysisClient) ListOpenIssues(label string, limit int) ([]ghadapter.Issue, error) {
	return append([]ghadapter.Issue(nil), f.openIssues...), nil
}

func (f *fakeDeepAnalysisClient) CreateIssue(title, body string, labels []string) (*ghadapter.Issue, error) {
	f.createCalls++
	f.lastTitle = title
	f.lastBody = body
	f.lastLabels = append([]string(nil), labels...)
	if f.createdIssue != nil {
		return f.createdIssue, nil
	}
	return &ghadapter.Issue{Repo: "41490/ccclaw", Number: 88, Title: title, Body: body}, nil
}

func TestShouldTriggerDeepAnalysisConsecutiveDays(t *testing.T) {
	now := time.Date(2026, 3, 12, 9, 0, 0, 0, time.Local)
	gaps := []GapSignal{
		{Keyword: "失败", Date: now.AddDate(0, 0, -3)},
		{Keyword: "失败", Date: now.AddDate(0, 0, -2)},
		{Keyword: "失败", Date: now.AddDate(0, 0, -1)},
	}
	if !ShouldTriggerDeepAnalysis(gaps) {
		t.Fatal("expected consecutive-day trigger")
	}
}

func TestShouldTriggerDeepAnalysisAccumulatedThreshold(t *testing.T) {
	now := time.Date(2026, 3, 12, 9, 0, 0, 0, time.Local)
	gaps := []GapSignal{
		{ID: "1", Keyword: "失败", Date: now},
		{ID: "2", Keyword: "找不到", Date: now},
		{ID: "3", Keyword: "失败", Date: now},
		{ID: "4", Keyword: "卡住", Date: now},
		{ID: "5", Keyword: "不会", Date: now},
	}
	if !ShouldTriggerDeepAnalysis(gaps) {
		t.Fatal("expected accumulated trigger")
	}
}

func TestMaybeTriggerDeepAnalysisCreatesIssue(t *testing.T) {
	now := time.Date(2026, 3, 12, 9, 0, 0, 0, time.Local)
	client := &fakeDeepAnalysisClient{
		createdIssue: &ghadapter.Issue{Repo: "41490/ccclaw", Number: 88, Title: "[sevolver] 能力缺口深度分析 2026-03-12"},
	}
	decision, err := MaybeTriggerDeepAnalysis(Config{
		Now:         now,
		ControlRepo: "41490/ccclaw",
		TargetRepo:  "41490/ccclaw",
		IssueLabel:  "ccclaw",
		IssueClient: client,
	}, []GapSignal{
		{ID: "gap-1", Keyword: "失败", Date: now.AddDate(0, 0, -3), Source: "journal/a.md", Context: "第一次失败"},
		{ID: "gap-2", Keyword: "失败", Date: now.AddDate(0, 0, -2), Source: "journal/b.md", Context: "第二次失败"},
		{ID: "gap-3", Keyword: "失败", Date: now.AddDate(0, 0, -1), Source: "journal/c.md", Context: "第三次失败"},
	})
	if err != nil {
		t.Fatalf("MaybeTriggerDeepAnalysis failed: %v", err)
	}
	if decision == nil || !decision.Created || decision.IssueNumber != 88 {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if client.createCalls != 1 {
		t.Fatalf("expected single create call, got %d", client.createCalls)
	}
	if !strings.Contains(client.lastBody, "target_repo: 41490/ccclaw") {
		t.Fatalf("expected target repo in issue body: %s", client.lastBody)
	}
	if !strings.Contains(client.lastBody, "gap-1") {
		t.Fatalf("expected gap details in issue body: %s", client.lastBody)
	}
	if len(client.lastLabels) != 1 || client.lastLabels[0] != "ccclaw" {
		t.Fatalf("unexpected labels: %#v", client.lastLabels)
	}
}

func TestMaybeTriggerDeepAnalysisReusesExistingOpenIssue(t *testing.T) {
	now := time.Date(2026, 3, 12, 9, 0, 0, 0, time.Local)
	plans := buildDeepAnalysisPlans([]GapSignal{
		{ID: "gap-1", Keyword: "失败", Date: now.AddDate(0, 0, -3)},
		{ID: "gap-2", Keyword: "失败", Date: now.AddDate(0, 0, -2)},
		{ID: "gap-3", Keyword: "失败", Date: now.AddDate(0, 0, -1)},
	})
	if len(plans) == 0 {
		t.Fatal("expected plans")
	}
	client := &fakeDeepAnalysisClient{
		openIssues: []ghadapter.Issue{{
			Repo:   "41490/ccclaw",
			Number: 99,
			Title:  "[sevolver] 能力缺口深度分析 2026-03-11",
			Body:   renderDeepAnalysisIssueBody(Config{Now: now, TargetRepo: "41490/ccclaw"}, plans[0], plans[0].Gaps),
		}},
	}
	decision, err := MaybeTriggerDeepAnalysis(Config{
		Now:         now,
		ControlRepo: "41490/ccclaw",
		TargetRepo:  "41490/ccclaw",
		IssueLabel:  "ccclaw",
		IssueClient: client,
	}, []GapSignal{
		{ID: "gap-1", Keyword: "失败", Date: now.AddDate(0, 0, -3)},
		{ID: "gap-2", Keyword: "失败", Date: now.AddDate(0, 0, -2)},
		{ID: "gap-3", Keyword: "失败", Date: now.AddDate(0, 0, -1)},
	})
	if err != nil {
		t.Fatalf("MaybeTriggerDeepAnalysis failed: %v", err)
	}
	if decision == nil || !decision.Existing || decision.Created {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if client.createCalls != 0 {
		t.Fatalf("expected no create call, got %d", client.createCalls)
	}
}
