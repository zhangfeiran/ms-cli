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
	Stdout        string
	Stderr        string
	ReturnCode    int
	ExceptionInfo string
}

// Tool wraps shell execution for runtime.
// Each call runs in a fresh shell process.
type Tool struct {
	Cwd string
	Env map[string]string
}

// Run executes one command in `bash -c`.
// We intentionally avoid `-l` to prevent user profile/login scripts from
// muting or altering stdout/stderr in non-interactive runs.
func (t Tool) Run(ctx context.Context, command string) Result {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	if t.Cwd != "" {
		cmd.Dir = t.Cwd
	}
	cmd.Env = mergeEnv(os.Environ(), t.Env)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stdoutText := stdout.String()
		stderrText := stderr.String()
		combined := combineOutputs(stdoutText, stderrText)

		if ctx.Err() != nil {
			return Result{
				Output:        combined,
				Stdout:        stdoutText,
				Stderr:        stderrText,
				ReturnCode:    -1,
				ExceptionInfo: fmt.Sprintf("command timed out or canceled: %v", ctx.Err()),
			}
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			return Result{
				Output:        combined,
				Stdout:        stdoutText,
				Stderr:        stderrText,
				ReturnCode:    exitErr.ExitCode(),
				ExceptionInfo: "",
			}
		}

		return Result{
			Output:        combined,
			Stdout:        stdoutText,
			Stderr:        stderrText,
			ReturnCode:    -1,
			ExceptionInfo: fmt.Sprintf("failed to execute command: %v", err),
		}
	}

	stdoutText := stdout.String()
	stderrText := stderr.String()

	return Result{
		Output:        combineOutputs(stdoutText, stderrText),
		Stdout:        stdoutText,
		Stderr:        stderrText,
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

func combineOutputs(stdout, stderr string) string {
	if stdout == "" {
		return stderr
	}
	if stderr == "" {
		return stdout
	}
	return stdout + "\n" + stderr
}
