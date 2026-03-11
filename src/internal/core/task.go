package core

import (
	"fmt"
	"strings"
	"time"
)

type State string

const (
	StateNew     State = "NEW"
	StateRunning State = "RUNNING"
	StateBlocked State = "BLOCKED"
	StateFailed  State = "FAILED"
	StateDone    State = "DONE"
	StateDead    State = "DEAD"
)

type EventType string

const (
	EventCreated EventType = "CREATED"
	EventBlocked EventType = "BLOCKED"
	EventStarted EventType = "STARTED"
	EventFailed  EventType = "FAILED"
	EventDone    EventType = "DONE"
	EventDead    EventType = "DEAD"
	EventUpdated EventType = "UPDATED"
	EventWarning EventType = "WARNING"
)

type RiskLevel string

const (
	RiskLow  RiskLevel = "low"
	RiskMed  RiskLevel = "med"
	RiskHigh RiskLevel = "high"
)

type Intent string

const (
	IntentResearch Intent = "research"
	IntentFix      Intent = "fix"
	IntentOps      Intent = "ops"
	IntentRelease  Intent = "release"
)

const MaxRetry = 3

type Task struct {
	TaskID                string
	IdempotencyKey        string
	ControlRepo           string
	IssueRepo             string
	TargetRepo            string
	LastSessionID         string
	IssueNumber           int
	IssueTitle            string
	IssueBody             string
	IssueAuthor           string
	IssueAuthorPermission string
	Labels                []string
	Intent                Intent
	RiskLevel             RiskLevel
	Approved              bool
	ApprovalCommand       string
	ApprovalActor         string
	ApprovalCommentID     int64
	State                 State
	RetryCount            int
	ErrorMsg              string
	ReportPath            string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type TokenUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

func IdempotencyKey(issueRepo string, issueNumber int) string {
	return fmt.Sprintf("%s#%d#body", strings.TrimSpace(issueRepo), issueNumber)
}

func TaskID(issueRepo string, issueNumber int) string {
	return IdempotencyKey(issueRepo, issueNumber)
}

func InferIntent(labels []string) Intent {
	for _, label := range labels {
		switch strings.ToLower(label) {
		case "ops":
			return IntentOps
		case "bug", "fix":
			return IntentFix
		case "release":
			return IntentRelease
		case "research":
			return IntentResearch
		}
	}
	return IntentResearch
}

func InferRisk(labels []string) RiskLevel {
	for _, label := range labels {
		switch strings.ToLower(label) {
		case "risk:high":
			return RiskHigh
		case "risk:med":
			return RiskMed
		}
	}
	return RiskLow
}

func SafeReportFileName(now time.Time, issueNumber int, title string) string {
	var cleaned strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			cleaned.WriteRune(r)
		case r == '-', r == '_':
			cleaned.WriteRune(r)
		default:
			if cleaned.Len() == 0 || strings.HasSuffix(cleaned.String(), "_") {
				continue
			}
			cleaned.WriteRune('_')
		}
	}
	name := strings.Trim(cleaned.String(), "_")
	if name == "" {
		name = "task_report"
	}
	return fmt.Sprintf("%s_%d_%s.md", now.Format("060102"), issueNumber, name)
}
