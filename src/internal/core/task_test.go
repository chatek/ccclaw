package core

import "testing"

func TestInferTaskClassUsesExplicitBodyMarker(t *testing.T) {
	got := InferTaskClass("[sevolver] 能力缺口深度分析 2026-03-12", "task_class: sevolver_deep_analysis\n", []string{"ccclaw"})
	if got != TaskClassSevolverDeepAnalysis {
		t.Fatalf("unexpected task class: %s", got)
	}
}

func TestInferTaskClassFallsBackToSevolverTitle(t *testing.T) {
	got := InferTaskClass("[sevolver] 日常整理", "普通说明", []string{"ccclaw"})
	if got != TaskClassSevolver {
		t.Fatalf("unexpected task class: %s", got)
	}
}

func TestInferTaskClassDefaultsToGeneral(t *testing.T) {
	got := InferTaskClass("修复状态页", "target_repo: 41490/ccclaw", []string{"ccclaw"})
	if got != TaskClassGeneral {
		t.Fatalf("unexpected task class: %s", got)
	}
}
