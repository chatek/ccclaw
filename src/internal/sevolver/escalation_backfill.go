package sevolver

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	gapEscalationStatusEscalated = "escalated"
	gapEscalationStatusResolved  = "resolved"
	gapEscalationStatusConverged = "converged"
)

type DeepAnalysisBackfillResult struct {
	GapIDs     []string
	SkillPaths []string
}

type DeepAnalysisResolutionResult struct {
	GapIDs       []string
	SkillPaths   []string
	IssueNumbers []int
}

func BackfillDeepAnalysisEscalation(kbDir string, backlog []GapSignal, decision *DeepAnalysisDecision, hits []SkillHit, now time.Time) (*DeepAnalysisBackfillResult, error) {
	if decision == nil || !decision.Triggered || decision.IssueNumber <= 0 {
		return &DeepAnalysisBackfillResult{}, nil
	}

	targetIDs := map[string]struct{}{}
	for _, gap := range decision.RelevantGaps {
		if strings.TrimSpace(gap.ID) == "" {
			continue
		}
		targetIDs[gap.ID] = struct{}{}
	}
	if len(targetIDs) == 0 {
		return &DeepAnalysisBackfillResult{}, nil
	}

	sourceSkills := indexSkillHitsBySource(hits)
	updatedGapIDs := make([]string, 0, len(targetIDs))
	gapIDsBySkill := map[string][]string{}
	updatedBacklog := append([]GapSignal(nil), backlog...)
	for idx := range updatedBacklog {
		gap := &updatedBacklog[idx]
		if _, ok := targetIDs[gap.ID]; !ok {
			continue
		}
		if len(gap.RelatedSkills) == 0 {
			gap.RelatedSkills = append([]string(nil), sourceSkills[gap.Source]...)
		}
		gap.RelatedSkills = uniqueSortedStrings(gap.RelatedSkills)
		gap.EscalationStatus = gapEscalationStatusEscalated
		gap.EscalationFingerprint = decision.Fingerprint
		gap.EscalationIssueNumber = decision.IssueNumber
		gap.EscalationIssueURL = decision.IssueURL
		gap.EscalationUpdatedAt = dateFloor(now).Format("2006-01-02")
		updatedGapIDs = append(updatedGapIDs, gap.ID)
		for _, skillPath := range gap.RelatedSkills {
			gapIDsBySkill[skillPath] = append(gapIDsBySkill[skillPath], gap.ID)
		}
	}
	sort.Strings(updatedGapIDs)

	if _, err := SaveGapSignals(kbDir, updatedBacklog, now); err != nil {
		return nil, err
	}

	updatedSkills := make([]string, 0, len(gapIDsBySkill))
	for skillPath, gapIDs := range gapIDsBySkill {
		if strings.TrimSpace(skillPath) == "" {
			continue
		}
		skillFile := filepath.Join(strings.TrimSpace(kbDir), filepath.FromSlash(skillPath))
		if err := ApplySkillGapEscalation(skillFile, gapIDs, *decision, now); err != nil {
			return nil, fmt.Errorf("回写 skill 缺口升级状态失败 %s: %w", filepath.ToSlash(skillFile), err)
		}
		updatedSkills = append(updatedSkills, filepath.ToSlash(skillPath))
	}
	sort.Strings(updatedSkills)

	return &DeepAnalysisBackfillResult{
		GapIDs:     updatedGapIDs,
		SkillPaths: updatedSkills,
	}, nil
}

func indexSkillHitsBySource(hits []SkillHit) map[string][]string {
	indexed := map[string][]string{}
	for _, hit := range hits {
		source := strings.TrimSpace(hit.Source)
		skillPath := strings.TrimSpace(hit.SkillPath)
		if source == "" || skillPath == "" {
			continue
		}
		indexed[source] = append(indexed[source], skillPath)
	}
	for source, paths := range indexed {
		indexed[source] = uniqueSortedStrings(paths)
	}
	return indexed
}

func ResolveClosedDeepAnalysisEscalations(cfg Config, backlog []GapSignal, now time.Time) ([]GapSignal, *DeepAnalysisResolutionResult, error) {
	updatedBacklog := cloneGapSignals(backlog)
	result := &DeepAnalysisResolutionResult{}
	if len(updatedBacklog) == 0 {
		return updatedBacklog, result, nil
	}

	issueNumbers := collectEscalatedIssueNumbers(updatedBacklog)
	if len(issueNumbers) == 0 {
		return updatedBacklog, result, nil
	}

	client, err := deepAnalysisClientForConfig(cfg)
	if err != nil {
		return updatedBacklog, result, err
	}
	if client == nil {
		return updatedBacklog, result, nil
	}

	closedIssues := map[int]struct{}{}
	for _, issueNumber := range issueNumbers {
		issue, err := client.GetIssue(issueNumber)
		if err != nil {
			return updatedBacklog, result, fmt.Errorf("读取 deep-analysis issue #%d 失败: %w", issueNumber, err)
		}
		if issue == nil || !strings.EqualFold(strings.TrimSpace(issue.State), "closed") {
			continue
		}
		closedIssues[issueNumber] = struct{}{}
		result.IssueNumbers = append(result.IssueNumbers, issueNumber)
	}
	if len(closedIssues) == 0 {
		return updatedBacklog, result, nil
	}
	sort.Ints(result.IssueNumbers)

	resolutionTargets := map[string]skillGapEscalation{}
	for idx := range updatedBacklog {
		gap := &updatedBacklog[idx]
		if !strings.EqualFold(strings.TrimSpace(gap.EscalationStatus), gapEscalationStatusEscalated) {
			continue
		}
		if _, ok := closedIssues[gap.EscalationIssueNumber]; !ok {
			continue
		}
		gap.EscalationStatus = gapEscalationStatusResolved
		gap.EscalationUpdatedAt = dateFloor(now).Format("2006-01-02")
		result.GapIDs = append(result.GapIDs, gap.ID)
		key := resolutionKey(gap.EscalationFingerprint, gap.EscalationIssueNumber)
		target := resolutionTargets[key]
		target.Fingerprint = strings.TrimSpace(gap.EscalationFingerprint)
		target.IssueNumber = gap.EscalationIssueNumber
		target.GapIDs = append(target.GapIDs, gap.ID)
		resolutionTargets[key] = target
	}
	if len(result.GapIDs) == 0 {
		return updatedBacklog, result, nil
	}
	sort.Strings(result.GapIDs)

	if _, err := SaveGapSignals(cfg.KBDir, updatedBacklog, now); err != nil {
		return updatedBacklog, result, err
	}

	targets := make([]skillGapEscalation, 0, len(resolutionTargets))
	for _, item := range resolutionTargets {
		item.Status = gapEscalationStatusConverged
		item.UpdatedAt = dateFloor(now).Format("2006-01-02")
		item.GapIDs = uniqueSortedStrings(item.GapIDs)
		targets = append(targets, item)
	}
	sort.Slice(targets, func(i, j int) bool {
		return resolutionKey(targets[i].Fingerprint, targets[i].IssueNumber) < resolutionKey(targets[j].Fingerprint, targets[j].IssueNumber)
	})

	updatedSkills, err := resolveSkillGapEscalations(cfg.KBDir, targets, now)
	if err != nil {
		return updatedBacklog, result, err
	}
	result.SkillPaths = updatedSkills
	return updatedBacklog, result, nil
}

func filterActiveGapSignals(items []GapSignal) []GapSignal {
	active := make([]GapSignal, 0, len(items))
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.EscalationStatus), gapEscalationStatusResolved) {
			continue
		}
		active = append(active, item)
	}
	sortGapSignals(active)
	return active
}

func collectEscalatedIssueNumbers(backlog []GapSignal) []int {
	seen := map[int]struct{}{}
	numbers := make([]int, 0)
	for _, gap := range backlog {
		if !strings.EqualFold(strings.TrimSpace(gap.EscalationStatus), gapEscalationStatusEscalated) {
			continue
		}
		if gap.EscalationIssueNumber <= 0 {
			continue
		}
		if _, ok := seen[gap.EscalationIssueNumber]; ok {
			continue
		}
		seen[gap.EscalationIssueNumber] = struct{}{}
		numbers = append(numbers, gap.EscalationIssueNumber)
	}
	sort.Ints(numbers)
	return numbers
}

func resolveSkillGapEscalations(kbDir string, targets []skillGapEscalation, now time.Time) ([]string, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	skillsRoot := filepath.Join(strings.TrimSpace(kbDir), "skills")
	if _, err := os.Stat(skillsRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取 skills 目录失败: %w", err)
	}
	updated := make([]string, 0)
	err := filepath.WalkDir(skillsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		changed, err := ApplySkillGapEscalationResolution(path, targets, now)
		if err != nil {
			return fmt.Errorf("回写 skill 缺口收敛状态失败 %s: %w", filepath.ToSlash(path), err)
		}
		if changed {
			rel, relErr := filepath.Rel(strings.TrimSpace(kbDir), path)
			if relErr != nil {
				updated = append(updated, filepath.ToSlash(path))
				return nil
			}
			updated = append(updated, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(updated)
	return updated, nil
}

func resolutionKey(fingerprint string, issueNumber int) string {
	return strings.TrimSpace(fingerprint) + "|" + fmt.Sprintf("%09d", issueNumber)
}
