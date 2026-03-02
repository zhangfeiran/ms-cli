package shell

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
)

// Result is the standardized output of one shell execution.
type Result struct {
	Output        string
	ReturnCode    int
	ExceptionInfo string
}

// Tool wraps shell execution for runtime.
// Each call runs in a fresh shell process.
type Tool struct {
	Cwd string
	Env map[string]string
}

// Run executes one command in `bash -lc`.
func (t Tool) Run(ctx context.Context, command string) Result {
	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	if t.Cwd != "" {
		cmd.Dir = t.Cwd
	}
	cmd.Env = mergeEnv(os.Environ(), t.Env)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Result{
				Output:        out.String(),
				ReturnCode:    -1,
				ExceptionInfo: fmt.Sprintf("command timed out or canceled: %v", ctx.Err()),
			}
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			return Result{
				Output:        out.String(),
				ReturnCode:    exitErr.ExitCode(),
				ExceptionInfo: "",
			}
		}

		return Result{
			Output:        out.String(),
			ReturnCode:    -1,
			ExceptionInfo: fmt.Sprintf("failed to execute command: %v", err),
		}
	}

	return Result{
		Output:        out.String(),
		ReturnCode:    0,
		ExceptionInfo: "",
	}
}

func mergeEnv(base []string, extra map[string]string) []string {
	if len(extra) == 0 {
		return base
	}

	env := make([]string, 0, len(base)+len(extra))
	env = append(env, base...)

	// Stable order keeps behavior deterministic in tests/logging.
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, fmt.Sprintf("%s=%s", k, extra[k]))
	}

	return env
}
