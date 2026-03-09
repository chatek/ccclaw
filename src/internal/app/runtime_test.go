package app

import (
	"testing"

	"github.com/41490/ccclaw/internal/config"
)

func TestParseTargetRepo(t *testing.T) {
	body := "请处理这个问题\n\ntarget_repo: 41490/ccclaw\n"
	if got := parseTargetRepo(body); got != "41490/ccclaw" {
		t.Fatalf("unexpected target repo: %q", got)
	}
}

func TestResolveTargetRepoFallsBackToDefaultTarget(t *testing.T) {
	rt := &Runtime{
		cfg: &config.Config{
			DefaultTarget: "41490/ccclaw",
			Paths:         config.PathsConfig{KBDir: "/opt/ccclaw/kb"},
			Targets: []config.TargetConfig{{
				Repo:      "41490/ccclaw",
				LocalPath: "/opt/src/ccclaw",
			}},
		},
	}
	repo, reasons := rt.resolveTargetRepo("无显式 target")
	if repo != "41490/ccclaw" {
		t.Fatalf("unexpected repo: %q", repo)
	}
	if len(reasons) != 0 {
		t.Fatalf("expected no reasons, got %#v", reasons)
	}
}

func TestResolveTargetRepoBlocksWhenNoTargetConfigured(t *testing.T) {
	rt := &Runtime{
		cfg: &config.Config{
			Paths:   config.PathsConfig{KBDir: "/opt/ccclaw/kb"},
			Targets: nil,
		},
	}
	repo, reasons := rt.resolveTargetRepo("无显式 target")
	if repo != "" {
		t.Fatalf("expected empty repo, got %q", repo)
	}
	if len(reasons) != 1 {
		t.Fatalf("expected 1 reason, got %#v", reasons)
	}
}
