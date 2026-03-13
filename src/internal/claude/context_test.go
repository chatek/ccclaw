package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContextMetricsFromTranscriptUsesLatestMainlineUsageAndUsableWindow(t *testing.T) {
	transcriptPath := filepath.Join(t.TempDir(), "transcript.jsonl")
	content := strings.Join([]string{
		`{"timestamp":"2026-03-12T10:00:00Z","message":{"model":"Claude Sonnet 4","usage":{"input_tokens":100,"cache_creation_input_tokens":50,"cache_read_input_tokens":25}}}`,
		`{"timestamp":"2026-03-12T10:01:00Z","isSidechain":true,"message":{"model":"Claude Sonnet 4","usage":{"input_tokens":9999}}}`,
		`{"timestamp":"2026-03-12T10:02:00Z","message":{"model":"Claude Sonnet 4","usage":{"input_tokens":80000,"cache_creation_input_tokens":20000,"cache_read_input_tokens":12000}}}`,
	}, "\n")
	if err := os.WriteFile(transcriptPath, []byte(content), 0o644); err != nil {
		t.Fatalf("写入 transcript 失败: %v", err)
	}

	metrics, err := ContextMetricsFromTranscript(transcriptPath, "", 200000)
	if err != nil {
		t.Fatalf("ContextMetricsFromTranscript 失败: %v", err)
	}
	if metrics == nil {
		t.Fatal("expected metrics, got nil")
	}
	if metrics.ContextLength != 112000 {
		t.Fatalf("unexpected context length: %#v", metrics)
	}
	if metrics.UsableTokens != 160000 {
		t.Fatalf("unexpected usable tokens: %#v", metrics)
	}
	if metrics.UsablePercent != 70 {
		t.Fatalf("unexpected usable percent: %#v", metrics)
	}
}
