package scheduler

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
				Level:         "info",
				ArchiveDir:    "/tmp/ccclaw-app/log/scheduler",
				RetentionDays: 30,
				MaxFiles:      200,
				Compress:      true,
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
	if !strings.Contains(units["ccclaw-archive.timer"], "OnCalendar=Mon 02:00:00 Asia/Shanghai") {
		t.Fatalf("unexpected archive timer unit: %q", units["ccclaw-archive.timer"])
	}
	if !strings.Contains(units["ccclaw-sevolver.timer"], "OnCalendar=*-*-* 22:00:00 Asia/Shanghai") {
		t.Fatalf("unexpected sevolver timer unit: %q", units["ccclaw-sevolver.timer"])
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

func TestApplyLogArchivePolicyCompressesAndTrimsManagedFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	currentPath := BuildLogArchivePath(dir, "run", now)
	createManagedArchiveFixture(t, currentPath, now, "current\n")
	keepPath := filepath.Join(dir, "260311_010203_run.log")
	createManagedArchiveFixture(t, keepPath, now.Add(-24*time.Hour), "keep\n")
	trimPath := filepath.Join(dir, "260310_010203_run.log")
	createManagedArchiveFixture(t, trimPath, now.Add(-48*time.Hour), "trim\n")
	expiredPath := filepath.Join(dir, "260101_010203_run.log")
	createManagedArchiveFixture(t, expiredPath, now.Add(-40*24*time.Hour), "expired\n")
	manualPath := filepath.Join(dir, "260309_010203_run.log")
	if err := os.WriteFile(manualPath, []byte("manual\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyLogArchivePolicy(dir, currentPath, LogArchivePolicy{
		RetentionDays: 30,
		MaxFiles:      2,
		Compress:      true,
	}); err != nil {
		t.Fatalf("apply archive policy failed: %v", err)
	}
	if _, err := os.Stat(currentPath); err != nil {
		t.Fatalf("expected current archive to stay uncompressed: %v", err)
	}
	if _, err := os.Stat(keepPath + ".gz"); err != nil {
		t.Fatalf("expected keep archive to be compressed: %v", err)
	}
	if _, err := os.Stat(trimPath); !os.IsNotExist(err) {
		t.Fatalf("expected trimmed archive to be deleted, got err=%v", err)
	}
	if _, err := os.Stat(trimPath + ".gz"); !os.IsNotExist(err) {
		t.Fatalf("expected trimmed compressed archive to be deleted, got err=%v", err)
	}
	if _, err := os.Stat(expiredPath); !os.IsNotExist(err) {
		t.Fatalf("expected expired archive to be deleted, got err=%v", err)
	}
	if _, err := os.Stat(expiredPath + ".gz"); !os.IsNotExist(err) {
		t.Fatalf("expected expired compressed archive to be deleted, got err=%v", err)
	}
	if _, err := os.Stat(manualPath); err != nil {
		t.Fatalf("expected manual file to stay untouched: %v", err)
	}
	files, err := listManagedArchiveFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 managed archives after trimming, got %d", len(files))
	}
}

func createManagedArchiveFixture(t *testing.T, path string, modTime time.Time, body string) {
	t.Helper()

	handle, err := openLogArchive(LogsOptions{
		Scope:       "run",
		ArchivePath: path,
	})
	if err != nil {
		t.Fatalf("create archive fixture failed: %v", err)
	}
	if _, err := handle.WriteString(body); err != nil {
		_ = handle.Close()
		t.Fatalf("write archive fixture failed: %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("close archive fixture failed: %v", err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("set archive fixture time failed: %v", err)
	}
}
