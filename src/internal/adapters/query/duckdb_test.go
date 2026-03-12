package query

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDuckDBQueryAndCopy(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "duckdb.log")
	scriptPath := filepath.Join(dir, "duckdb")
	script := `#!/usr/bin/env bash
set -euo pipefail
log_file="${CCCLAW_FAKE_DUCKDB_LOG:?}"
printf '%s\n' "$*" >> "$log_file"
json_mode=0
sql=""
while (($#)); do
  case "$1" in
    -json) json_mode=1 ;;
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
if [[ "$json_mode" -eq 1 ]]; then
  printf '[{"ok":true}]'
else
  printf 'ok\n'
fi
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CCCLAW_FAKE_DUCKDB_LOG", logPath)

	client := NewWithBinary(scriptPath)
	rows, err := client.Query(context.Background(), "select 1 as ok")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(rows) != 1 || rows[0]["ok"] != true {
		t.Fatalf("unexpected rows: %#v", rows)
	}

	target := filepath.Join(dir, "archive", "events.parquet")
	if err := client.CopyJSONLToParquet(context.Background(), filepath.Join(dir, "events.jsonl"), target); err != nil {
		t.Fatalf("copy failed: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected parquet output: %v", err)
	}
	payload, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "-json -c select 1 as ok") {
		t.Fatalf("unexpected query args: %q", string(payload))
	}
	if !strings.Contains(string(payload), "COMPRESSION ZSTD") {
		t.Fatalf("unexpected copy args: %q", string(payload))
	}
}
