package shell

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestToolRunCapturesStdoutAndStderr(t *testing.T) {
	t.Parallel()

	tool := Tool{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := tool.Run(ctx, "echo STDOUT_TEST; echo STDERR_TEST >&2")
	if result.ReturnCode != 0 {
		t.Fatalf("expected returncode 0, got %d, exception=%s", result.ReturnCode, result.ExceptionInfo)
	}
	if !strings.Contains(result.Stdout, "STDOUT_TEST") {
		t.Fatalf("stdout missing expected text: %q", result.Stdout)
	}
	if !strings.Contains(result.Stderr, "STDERR_TEST") {
		t.Fatalf("stderr missing expected text: %q", result.Stderr)
	}
	if !strings.Contains(result.Output, "STDOUT_TEST") || !strings.Contains(result.Output, "STDERR_TEST") {
		t.Fatalf("combined output missing expected text: %q", result.Output)
	}
}
