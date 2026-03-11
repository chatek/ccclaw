package scheduler

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/41490/ccclaw/internal/config"
)

func testSystemdConfig() *config.Config {
	return &config.Config{
		Paths: config.PathsConfig{
			AppDir:   "/tmp/ccclaw-app",
			EnvFile:  "/tmp/ccclaw-app/.env",
			HomeRepo: "/opt/ccclaw",
			StateDB:  "/tmp/ccclaw-app/var/state.db",
			LogDir:   "/tmp/ccclaw-app/log",
			KBDir:    "/opt/ccclaw/kb",
		},
		Scheduler: config.SchedulerConfig{
			Mode:             "systemd",
			SystemdUserDir:   "/tmp/systemd-user",
			CalendarTimezone: "Asia/Shanghai",
			Timers: config.SchedulerTimersConfig{
				Ingest:  "*:0/5",
				Run:     "*:0/10",
				Patrol:  "*:0/2",
				Journal: "*-*-* 01:01:42",
			},
			Logs: config.SchedulerLogsConfig{
				Level:      "info",
				ArchiveDir: "/tmp/ccclaw-app/log/scheduler",
			},
		},
	}
}

func TestGenerateSystemdUnitContentsIncludesCalendarTimezone(t *testing.T) {
	units, err := GenerateSystemdUnitContents(testSystemdConfig())
	if err != nil {
		t.Fatalf("generate units failed: %v", err)
	}
	if !strings.Contains(units["ccclaw-journal.timer"], "OnCalendar=*-*-* 01:01:42 Asia/Shanghai") {
		t.Fatalf("unexpected journal timer unit: %q", units["ccclaw-journal.timer"])
	}
}

func TestStreamLogsBuildsExpectedJournalctlArgs(t *testing.T) {
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "bin")
	logFile := filepath.Join(dir, "journalctl.log")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" > "${CCCLAW_FAKE_JOURNALCTL_LOG:?}"
printf 'ok\n'
`
	scriptPath := filepath.Join(fakeBin, "journalctl")
	if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CCCLAW_FAKE_JOURNALCTL_LOG", logFile)

	var out bytes.Buffer
	err := StreamLogs(context.Background(), LogsOptions{
		Scope:  "ingest",
		Follow: true,
		Since:  "1 hour ago",
		Lines:  20,
		Level:  "warning",
	}, &out, &out)
	if err != nil {
		t.Fatalf("stream logs failed: %v", err)
	}
	payload, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	for _, want := range []string{
		"--user --no-pager -n 20 -p warning --since 1 hour ago -f -u ccclaw-ingest.service",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in %q", want, text)
		}
	}
}

func TestStreamLogsArchivesToFile(t *testing.T) {
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "bin")
	logFile := filepath.Join(dir, "journalctl.log")
	archivePath := filepath.Join(dir, "archives", "run.log")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `#!/usr/bin/env bash
set -euo pipefail
printf 'line-1\nline-2\n'
`
	scriptPath := filepath.Join(fakeBin, "journalctl")
	if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CCCLAW_FAKE_JOURNALCTL_LOG", logFile)

	var out bytes.Buffer
	err := StreamLogs(context.Background(), LogsOptions{
		Scope:       "run",
		Lines:       10,
		ArchivePath: archivePath,
	}, &out, &out)
	if err != nil {
		t.Fatalf("stream logs failed: %v", err)
	}
	archivePayload, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(archivePayload)
	if !strings.Contains(text, "# ccclaw scheduler logs archive") || !strings.Contains(text, "line-1") {
		t.Fatalf("unexpected archive payload: %q", text)
	}
}
