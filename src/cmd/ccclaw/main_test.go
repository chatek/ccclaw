package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmdVersionFlag(t *testing.T) {
	cmd := newRootCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"-V"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("执行 -V 失败: %v", err)
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Fatal("预期输出版本号，实际为空")
	}
}

func TestRootCmdHelpByDefault(t *testing.T) {
	cmd := newRootCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("执行默认帮助失败: %v", err)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("预期输出帮助信息，实际为: %q", out.String())
	}
}
