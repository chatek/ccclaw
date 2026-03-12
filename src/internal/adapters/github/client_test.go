package github

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatchApprovalCommandSupportsAliasesAndCaseInsensitive(t *testing.T) {
	body := "请先评审\n/CCCLAW OK 现在可以继续\n谢谢"
	match, ok := MatchApprovalCommand(body, []string{"approve", "ok"}, []string{"reject"})
	if !ok {
		t.Fatal("expected approval command to be detected")
	}
	if !match.Approved || match.Rejected {
		t.Fatalf("unexpected match: %#v", match)
	}
	if match.Command != "/ccclaw ok" {
		t.Fatalf("unexpected command: %#v", match)
	}
}

func TestMatchApprovalCommandSupportsRejectWords(t *testing.T) {
	body := "/ccclaw 拒绝\n原因稍后补"
	match, ok := MatchApprovalCommand(body, []string{"approve"}, []string{"reject", "拒绝"})
	if !ok {
		t.Fatal("expected reject command to be detected")
	}
	if match.Approved || !match.Rejected {
		t.Fatalf("unexpected match: %#v", match)
	}
	if match.Command != "/ccclaw 拒绝" {
		t.Fatalf("unexpected command: %#v", match)
	}
}

func TestFindApprovalPrefersLatestTrustedCommand(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatalf("创建 fake bin 失败: %v", err)
	}

	script := filepath.Join(fakeBin, "gh")
	body := `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" != "api" ]]; then
  exit 1
fi
endpoint="${2:-}"
case "$endpoint" in
  "repos/41490/ccclaw/issues/15/comments?per_page=100")
    cat <<'JSON'
[
  {"id":101,"body":"/ccclaw approve","user":{"login":"outsider"}},
  {"id":102,"body":"准备推进\n/ccclaw approve","user":{"login":"maintainer"}},
  {"id":103,"body":"/CCCLAW reject","user":{"login":"maintainer"}}
]
JSON
    ;;
  "repos/41490/ccclaw/collaborators/outsider/permission")
    printf '{"permission":"read"}\n'
    ;;
  "repos/41490/ccclaw/collaborators/maintainer/permission")
    printf '{"permission":"maintain"}\n'
    ;;
  *)
    printf 'unexpected endpoint: %s\n' "$endpoint" >&2
    exit 1
    ;;
esac
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("写入 fake gh 失败: %v", err)
	}

	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+oldPath)

	client := NewClient("41490/ccclaw", map[string]string{})
	approval, err := client.FindApproval(15, []string{"approve", "go"}, []string{"reject", "拒绝"}, "maintain")
	if err != nil {
		t.Fatalf("FindApproval failed: %v", err)
	}
	if approval == nil {
		t.Fatal("expected approval result")
	}
	if approval.Approved || !approval.Rejected {
		t.Fatalf("unexpected approval result: %#v", approval)
	}
	if approval.Actor != "maintainer" || approval.CommentID != 103 {
		t.Fatalf("unexpected actor/comment: %#v", approval)
	}
	if approval.Command != "/ccclaw reject" {
		t.Fatalf("unexpected command: %#v", approval)
	}
}

func TestFindApprovalSkipsUnknownCommands(t *testing.T) {
	match, ok := MatchApprovalCommand("/ccclaw maybe", []string{"approve"}, []string{"reject"})
	if ok {
		t.Fatalf("did not expect unknown command: %#v", match)
	}
}

func TestIssueURL(t *testing.T) {
	if got := IssueURL("41490/ccclaw", 15); !strings.Contains(got, "/issues/15") {
		t.Fatalf("unexpected issue url: %s", got)
	}
}

func TestCreateIssuePassesLabelsAndParsesResponse(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatalf("创建 fake bin 失败: %v", err)
	}

	script := filepath.Join(fakeBin, "gh")
	body := `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" > "$TEST_GH_ARGS"
if [[ "${1:-}" != "api" ]]; then
  exit 1
fi
cat <<'JSON'
{"number":42,"title":"[sevolver] 能力缺口深度分析 2026-03-12","body":"demo","state":"open"}
JSON
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("写入 fake gh 失败: %v", err)
	}

	argsLog := filepath.Join(tmpDir, "args.log")
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+oldPath)
	t.Setenv("TEST_GH_ARGS", argsLog)

	client := NewClient("41490/ccclaw", map[string]string{})
	issue, err := client.CreateIssue("demo", "body", []string{"ccclaw", "sevolver"})
	if err != nil {
		t.Fatalf("CreateIssue failed: %v", err)
	}
	if issue == nil || issue.Number != 42 {
		t.Fatalf("unexpected issue: %#v", issue)
	}
	args, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("读取 gh 参数失败: %v", err)
	}
	text := string(args)
	if !strings.Contains(text, "labels[]=ccclaw") || !strings.Contains(text, "labels[]=sevolver") {
		t.Fatalf("expected labels in args, got: %s", text)
	}
}
