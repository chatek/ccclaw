package archive

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/41490/ccclaw/internal/adapters/storage"
	"github.com/41490/ccclaw/internal/core"
)

func TestRunAtExportsOlderWeeklyJSONL(t *testing.T) {
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "duckdb.log")
	scriptPath := filepath.Join(fakeBin, "duckdb")
	script := `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${CCCLAW_FAKE_DUCKDB_LOG:?}"
sql=""
while (($#)); do
  case "$1" in
    -c)
      shift
      sql="${1:-}"
      ;;
  esac
  shift || true
done
if [[ "$sql" =~ TO\ \'([^\']+)\' ]]; then
  target="${BASH_REMATCH[1]}"
  mkdir -p "$(dirname "$target")"
  printf 'parquet\n' > "$target"
fi
printf 'ok\n'
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CCCLAW_FAKE_DUCKDB_LOG", logPath)

	store, err := storage.Open(filepath.Join(dir, "var"))
	if err != nil {
		t.Fatal(err)
	}
	oldTime := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	currentTime := time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC)
	if err := store.AppendEventAt("task-old", core.EventCreated, "{}", oldTime); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordTokenUsage(storage.TokenUsageRecord{
		TaskID:     "task-old",
		SessionID:  "sess-old",
		Usage:      core.TokenUsage{InputTokens: 10, OutputTokens: 1},
		CostUSD:    0.1,
		RecordedAt: oldTime,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEventAt("task-current", core.EventCreated, "{}", currentTime); err != nil {
		t.Fatal(err)
	}

	result, err := RunAt(context.Background(), filepath.Join(dir, "var"), io.Discard, currentTime)
	if err != nil {
		t.Fatalf("archive run failed: %v", err)
	}
	if len(result.Archived) != 2 {
		t.Fatalf("expected 2 archived files, got %#v", result)
	}
	if len(result.Skipped) == 0 {
		t.Fatalf("expected current week skip, got %#v", result)
	}
	for _, name := range []string{"events-2026-W10.parquet", "token-2026-W10.parquet"} {
		if _, err := os.Stat(filepath.Join(dir, "var", "archive", name)); err != nil {
			t.Fatalf("expected archived parquet %s: %v", name, err)
		}
	}
}
