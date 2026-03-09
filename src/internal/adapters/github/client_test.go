package github

import "testing"

func TestHasApprovalCommand(t *testing.T) {
	body := "请先评审\n/ccclaw approve\n谢谢"
	if !HasApprovalCommand(body, "/ccclaw approve") {
		t.Fatal("expected approval command to be detected")
	}
	if HasApprovalCommand(body, "/ccclaw reject") {
		t.Fatal("did not expect unrelated command")
	}
}
