package sevolver

import (
	"crypto/sha1"
	"fmt"
	"sort"
	"strings"
	"time"

	ghadapter "github.com/41490/ccclaw/internal/adapters/github"
)

const (
	consecutiveDayThreshold    = 3
	accumulatedGapThreshold    = 5
	deepAnalysisIssueTitle     = "[sevolver] 能力缺口深度分析 %s"
	deepAnalysisMarkerTemplate = "<!-- ccclaw:sevolver:deep-analysis:fingerprint=%s -->"
	openIssueScanLimit         = 100
)

type DeepAnalysisClient interface {
	ListOpenIssues(label string, limit int) ([]ghadapter.Issue, error)
	CreateIssue(title, body string, labels []string) (*ghadapter.Issue, error)
}

type DeepAnalysisDecision struct {
	Triggered    bool
	Created      bool
	Existing     bool
	IssueNumber  int
	IssueURL     string
	Title        string
	Reason       string
	Fingerprint  string
	Keywords     []string
	GapCount     int
	BacklogCount int
	RelevantGaps []GapSignal
}

type deepAnalysisPlan struct {
	Fingerprint string
	Reason      string
	Keywords    []string
	Gaps        []GapSignal
}

type consecutiveCandidate struct {
	Keyword  string
	Start    time.Time
	End      time.Time
	GapCount int
	Gaps     []GapSignal
}

func ShouldTriggerDeepAnalysis(gaps []GapSignal) bool {
	return len(buildDeepAnalysisPlans(gaps)) > 0
}

func MaybeTriggerDeepAnalysis(cfg Config, gaps []GapSignal) (*DeepAnalysisDecision, error) {
	plans := buildDeepAnalysisPlans(gaps)
	decision := &DeepAnalysisDecision{
		Triggered:    len(plans) > 0,
		BacklogCount: len(gaps),
	}
	if len(plans) == 0 {
		decision.Reason = fmt.Sprintf("未达到阈值：当前未解决缺口 %d 条", len(gaps))
		return decision, nil
	}

	client, err := deepAnalysisClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("缺少 GitHub client，无法创建深度分析 issue")
	}

	openIssues, err := client.ListOpenIssues(strings.TrimSpace(cfg.IssueLabel), openIssueScanLimit)
	if err != nil {
		return nil, fmt.Errorf("读取 open issues 失败: %w", err)
	}
	existing := indexOpenDeepAnalysisIssues(openIssues)

	for idx := range plans {
		plan := plans[idx]
		if issue, ok := existing[plan.Fingerprint]; ok {
			copyIssue := issue
			return buildDeepAnalysisDecision(plan, &copyIssue, len(gaps), false, true), nil
		}
	}

	plan := plans[0]
	created, err := client.CreateIssue(
		fmt.Sprintf(deepAnalysisIssueTitle, dateFloor(cfg.deepAnalysisNow()).Format("2006-01-02")),
		renderDeepAnalysisIssueBody(cfg, plan, gaps),
		filterLabels(strings.TrimSpace(cfg.IssueLabel)),
	)
	if err != nil {
		return nil, err
	}
	if created == nil {
		return nil, fmt.Errorf("创建深度分析 issue 失败: 返回结果为空")
	}
	return buildDeepAnalysisDecision(plan, created, len(gaps), true, false), nil
}

func buildDeepAnalysisPlans(gaps []GapSignal) []deepAnalysisPlan {
	candidates := collectConsecutiveCandidates(gaps)
	plans := make([]deepAnalysisPlan, 0, len(candidates)+1)
	for _, candidate := range candidates {
		plans = append(plans, deepAnalysisPlan{
			Fingerprint: buildDeepAnalysisFingerprint("consecutive", candidate.Keyword),
			Reason:      fmt.Sprintf("同类缺口 `%s` 已连续 %d 天出现（%s -> %s）", candidate.Keyword, consecutiveDayThreshold, candidate.Start.Format("2006-01-02"), candidate.End.Format("2006-01-02")),
			Keywords:    []string{candidate.Keyword},
			Gaps:        candidate.Gaps,
		})
	}
	if len(gaps) >= accumulatedGapThreshold {
		plans = append(plans, deepAnalysisPlan{
			Fingerprint: buildDeepAnalysisFingerprint("accumulated"),
			Reason:      fmt.Sprintf("未解决缺口累计达到 %d 条", len(gaps)),
			Keywords:    uniqueGapKeywords(gaps),
			Gaps:        cloneGapSignals(gaps),
		})
	}
	return plans
}

func collectConsecutiveCandidates(gaps []GapSignal) []consecutiveCandidate {
	grouped := map[string][]GapSignal{}
	for _, gap := range gaps {
		keyword := strings.TrimSpace(gap.Keyword)
		if keyword == "" || gap.Date.IsZero() {
			continue
		}
		grouped[keyword] = append(grouped[keyword], gap)
	}

	candidates := make([]consecutiveCandidate, 0, len(grouped))
	for keyword, items := range grouped {
		dayMap := map[string]struct{}{}
		byDay := map[string][]GapSignal{}
		for _, item := range items {
			day := dateFloor(item.Date).Format("2006-01-02")
			dayMap[day] = struct{}{}
			byDay[day] = append(byDay[day], item)
		}
		days := make([]time.Time, 0, len(dayMap))
		for day := range dayMap {
			parsed, err := time.ParseInLocation("2006-01-02", day, time.Local)
			if err != nil {
				continue
			}
			days = append(days, dateFloor(parsed))
		}
		sort.Slice(days, func(i, j int) bool {
			return days[i].Before(days[j])
		})
		if len(days) < consecutiveDayThreshold {
			continue
		}

		bestLen := 0
		bestStart := time.Time{}
		bestEnd := time.Time{}
		startIdx := 0
		for idx := 1; idx <= len(days); idx++ {
			if idx < len(days) && calendarDaysBetween(days[idx], days[idx-1]) == 1 {
				continue
			}
			streakLen := idx - startIdx
			if streakLen >= consecutiveDayThreshold {
				start := days[startIdx]
				end := days[idx-1]
				if streakLen > bestLen || (streakLen == bestLen && end.After(bestEnd)) {
					bestLen = streakLen
					bestStart = start
					bestEnd = end
				}
			}
			startIdx = idx
		}
		if bestLen < consecutiveDayThreshold {
			continue
		}

		relevant := make([]GapSignal, 0)
		for day := bestStart; !day.After(bestEnd); day = day.AddDate(0, 0, 1) {
			relevant = append(relevant, byDay[day.Format("2006-01-02")]...)
		}
		sortGapSignals(relevant)
		candidates = append(candidates, consecutiveCandidate{
			Keyword:  keyword,
			Start:    bestStart,
			End:      bestEnd,
			GapCount: len(relevant),
			Gaps:     relevant,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		leftDays := calendarDaysBetween(candidates[i].End, candidates[i].Start) + 1
		rightDays := calendarDaysBetween(candidates[j].End, candidates[j].Start) + 1
		switch {
		case leftDays != rightDays:
			return leftDays > rightDays
		case !candidates[i].End.Equal(candidates[j].End):
			return candidates[i].End.After(candidates[j].End)
		case candidates[i].GapCount != candidates[j].GapCount:
			return candidates[i].GapCount > candidates[j].GapCount
		default:
			return candidates[i].Keyword < candidates[j].Keyword
		}
	})
	return candidates
}

func renderDeepAnalysisIssueBody(cfg Config, plan deepAnalysisPlan, backlog []GapSignal) string {
	targetRepo := strings.TrimSpace(cfg.TargetRepo)
	if targetRepo == "" {
		targetRepo = strings.TrimSpace(cfg.ControlRepo)
	}
	lines := []string{
		fmt.Sprintf(deepAnalysisMarkerTemplate, plan.Fingerprint),
		fmt.Sprintf("target_repo: %s", targetRepo),
		"",
		"## 背景",
		fmt.Sprintf("- 触发日期: %s", dateFloor(cfg.deepAnalysisNow()).Format("2006-01-02")),
		fmt.Sprintf("- 触发原因: %s", plan.Reason),
		fmt.Sprintf("- 当前未解决缺口总数: %d", len(backlog)),
		fmt.Sprintf("- 本次聚合关键词: %s", strings.Join(plan.Keywords, ", ")),
		fmt.Sprintf("- 指纹: `%s`", plan.Fingerprint),
		"",
		"## 触发样本",
	}
	for _, gap := range plan.Gaps {
		lines = append(lines, fmt.Sprintf("- [%s] %s | %s | %s | %s", gap.ID, gap.Date.Format("2006-01-02"), gap.Keyword, gap.Source, gap.Context))
	}
	lines = append(lines,
		"",
		"## 任务要求",
		"1. 先核对 Issue #32 已拍板的 Phase 3 规格与当前实现边界。",
		"2. 分析这些 gap signal 是否指向同一根因，禁止只做表面补丁。",
		"3. 如需代码修复，直接在 `main` 实施，并补齐测试。",
		"4. 如需知识库调整，补充或修订 `kb/skills/`、`kb/assay/` 相关资产。",
		"5. 完成后补 `docs/reports/` 工程报告，并回写结论。",
	)
	return strings.Join(lines, "\n")
}

func buildDeepAnalysisDecision(plan deepAnalysisPlan, issue *ghadapter.Issue, backlogCount int, created, existing bool) *DeepAnalysisDecision {
	decision := &DeepAnalysisDecision{
		Triggered:    true,
		Created:      created,
		Existing:     existing,
		Reason:       plan.Reason,
		Fingerprint:  plan.Fingerprint,
		Keywords:     append([]string(nil), plan.Keywords...),
		GapCount:     len(plan.Gaps),
		BacklogCount: backlogCount,
		RelevantGaps: cloneGapSignals(plan.Gaps),
	}
	if issue != nil {
		decision.Title = issue.Title
		decision.IssueNumber = issue.Number
		decision.IssueURL = ghadapter.IssueURL(issue.Repo, issue.Number)
	}
	return decision
}

func indexOpenDeepAnalysisIssues(issues []ghadapter.Issue) map[string]ghadapter.Issue {
	indexed := make(map[string]ghadapter.Issue, len(issues))
	for _, issue := range issues {
		fingerprint := extractDeepAnalysisFingerprint(issue.Body)
		if fingerprint == "" {
			continue
		}
		indexed[fingerprint] = issue
	}
	return indexed
}

func extractDeepAnalysisFingerprint(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		prefix := "<!-- ccclaw:sevolver:deep-analysis:fingerprint="
		if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, "-->") {
			continue
		}
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, prefix), "-->"))
	}
	return ""
}

func buildDeepAnalysisFingerprint(parts ...string) string {
	sum := sha1.Sum([]byte(strings.Join(parts, "|")))
	return fmt.Sprintf("sg-%x", sum[:6])
}

func deepAnalysisClientForConfig(cfg Config) (DeepAnalysisClient, error) {
	if cfg.IssueClient != nil {
		return cfg.IssueClient, nil
	}
	if strings.TrimSpace(cfg.ControlRepo) == "" {
		return nil, nil
	}
	return ghadapter.NewClient(cfg.ControlRepo, cfg.Secrets), nil
}

func filterLabels(labels ...string) []string {
	filtered := make([]string, 0, len(labels))
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		filtered = append(filtered, label)
	}
	return filtered
}

func uniqueGapKeywords(gaps []GapSignal) []string {
	seen := map[string]struct{}{}
	keywords := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		keyword := strings.TrimSpace(gap.Keyword)
		if keyword == "" {
			continue
		}
		if _, ok := seen[keyword]; ok {
			continue
		}
		seen[keyword] = struct{}{}
		keywords = append(keywords, keyword)
	}
	sort.Strings(keywords)
	return keywords
}

func sortGapSignals(gaps []GapSignal) {
	sort.Slice(gaps, func(i, j int) bool {
		if !gaps[i].Date.Equal(gaps[j].Date) {
			return gaps[i].Date.Before(gaps[j].Date)
		}
		if gaps[i].Keyword != gaps[j].Keyword {
			return gaps[i].Keyword < gaps[j].Keyword
		}
		return gaps[i].ID < gaps[j].ID
	})
}

func cloneGapSignals(gaps []GapSignal) []GapSignal {
	cloned := append([]GapSignal(nil), gaps...)
	sortGapSignals(cloned)
	return cloned
}
