package app

import (
	"strings"
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

func TestSummarizeSchedulerNoneModePasses(t *testing.T) {
	detail, err := summarizeScheduler(schedulerProbe{
		Requested:  "none",
		CronReason: "未检测到受控 crontab 规则",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if detail == "" {
		t.Fatal("expected scheduler detail")
	}
}

func TestSummarizeSchedulerAutoDegradeFails(t *testing.T) {
	detail, err := summarizeScheduler(schedulerProbe{
		Requested:     "auto",
		SystemdReason: "user bus 不可用",
		CronReason:    "未检测到受控 crontab 规则",
	})
	if err == nil {
		t.Fatal("expected degrade error")
	}
	if detail == "" {
		t.Fatal("expected scheduler detail")
	}
}

func TestSummarizeSchedulerSystemdNeedsRepair(t *testing.T) {
	detail, err := summarizeScheduler(schedulerProbe{
		Requested:        "systemd",
		SystemdUserDir:   "/tmp/systemd-user",
		SystemdInstalled: true,
		SystemdReason:    "systemctl --user is-enabled ccclaw-ingest.timer 失败: disabled",
	})
	if err == nil {
		t.Fatal("expected systemd mismatch error")
	}
	if want := "repair=systemctl --user daemon-reload"; !strings.Contains(detail, want) {
		t.Fatalf("expected repair hint %q in %q", want, detail)
	}
}

func TestSummarizeSchedulerCronPasses(t *testing.T) {
	detail, err := summarizeScheduler(schedulerProbe{
		Requested:  "cron",
		CronActive: true,
		CronReason: "已检测到受控 crontab ingest/run 规则",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if want := "effective=cron"; !strings.Contains(detail, want) {
		t.Fatalf("expected %q in %q", want, detail)
	}
}

func TestSummarizeSchedulerDoubleSchedulingFails(t *testing.T) {
	_, err := summarizeScheduler(schedulerProbe{
		Requested:     "auto",
		SystemdActive: true,
		SystemdReason: "systemd active",
		CronActive:    true,
		CronReason:    "cron active",
	})
	if err == nil {
		t.Fatal("expected duplicate scheduling error")
	}
}
