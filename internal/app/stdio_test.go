package app

import (
	"strings"
	"testing"
)

func TestRunStdioRejectsInvalidApprovalMode(t *testing.T) {
	err := runStdio([]string{"--approval", "harness"})
	if err == nil || !strings.Contains(err.Error(), "无效的审批方式，仅支持 auto 或 web") {
		t.Fatalf("expected approval mode validation failure, got %v", err)
	}
}

// Valid modes must pass validation and fail later at the local dial only,
// proving no real config or socket was touched before the validation gate.
func TestRunStdioAcceptsApprovalModesBeforeDialing(t *testing.T) {
	for _, args := range [][]string{
		{"--approval", "web", "--device-dir", t.TempDir()},
		{"--approval=", "--device-dir", t.TempDir()},
		{"--device-dir", t.TempDir()},
	} {
		err := runStdio(args)
		if err == nil || !strings.Contains(err.Error(), "无法连接本机 stdio MCP") {
			t.Fatalf("args %v: expected local dial failure, got %v", args, err)
		}
	}
}
