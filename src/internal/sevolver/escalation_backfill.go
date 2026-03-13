package sevolver

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	gapEscalationStatusEscalated = "escalated"
)

type DeepAnalysisBackfillResult struct {
	GapIDs     []string
	SkillPaths []string
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
