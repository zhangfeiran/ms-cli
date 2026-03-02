package shell

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Config holds the shell runner configuration.
type Config struct {
	WorkDir        string
	Timeout        time.Duration
	AllowedCmds    []string // Whitelist (empty = allow all)
	BlockedCmds    []string // Blacklist
	RequireConfirm []string // Commands requiring confirmation
	Env            map[string]string
}

// Result is the result of a command execution.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Error    error
}

// Runner executes shell commands.
type Runner struct {
	config Config
}

// NewRunner creates a new shell runner.
func NewRunner(cfg Config) *Runner {
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &Runner{config: cfg}
}

// Run executes a command and returns the result.
func (r *Runner) Run(ctx context.Context, command string) (*Result, error) {
	// Check if command is allowed
	if reason := r.checkAllowed(command); reason != "" {
		return &Result{
			ExitCode: -1,
			Error:    fmt.Errorf("command not allowed: %s", reason),
		}, nil
	}

	// Create command
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = r.config.WorkDir

	// Set environment
	cmd.Env = os.Environ()
	for k, v := range r.config.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Run with timeout if context doesn't have one
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && r.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.config.Timeout)
		defer cancel()
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
		cmd.Dir = r.config.WorkDir
	}

	// Capture output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start command: %w", err)
	}

	// Read output
	var stdoutLines, stderrLines []string

	stdoutDone := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			stdoutLines = append(stdoutLines, scanner.Text())
		}
		close(stdoutDone)
	}()

	stderrDone := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			stderrLines = append(stderrLines, scanner.Text())
		}
		close(stderrDone)
	}()

	// Wait for output readers
	<-stdoutDone
	<-stderrDone

	// Wait for command
	err = cmd.Wait()

	result := &Result{
		Stdout:   strings.Join(stdoutLines, "\n"),
		Stderr:   strings.Join(stderrLines, "\n"),
		ExitCode: 0,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
			result.Error = err
		}
	}

	return result, nil
}

// IsDangerous checks if a command might be dangerous.
func (r *Runner) IsDangerous(command string) bool {
	dangerous := []string{
		"rm -rf /", "rm -rf ~", "rm -rf /*",
		"> /dev/sda", "mkfs.", "dd if=",
		":(){ :|:& };:", // fork bomb
	}

	lower := strings.ToLower(command)
	for _, d := range dangerous {
		if strings.Contains(lower, d) {
			return true
		}
	}

	// Check destructive rm commands
	if strings.HasPrefix(lower, "rm ") && strings.Contains(lower, "-rf") {
		return true
	}

	return false
}

// checkAllowed checks if a command is allowed.
func (r *Runner) checkAllowed(command string) string {
	cmd := strings.TrimSpace(command)
	lower := strings.ToLower(cmd)

	// Check blacklist
	for _, blocked := range r.config.BlockedCmds {
		if strings.Contains(lower, strings.ToLower(blocked)) {
			return fmt.Sprintf("matches blocked pattern: %s", blocked)
		}
	}

	// Check whitelist (if defined)
	if len(r.config.AllowedCmds) > 0 {
		allowed := false
		for _, allowedCmd := range r.config.AllowedCmds {
			if strings.HasPrefix(lower, strings.ToLower(allowedCmd)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return "not in allowed commands list"
		}
	}

	return ""
}

// RequiresConfirm checks if a command requires user confirmation.
func (r *Runner) RequiresConfirm(command string) bool {
	cmd := strings.TrimSpace(strings.ToLower(command))

	for _, prefix := range r.config.RequireConfirm {
		if strings.HasPrefix(cmd, strings.ToLower(prefix)) {
			return true
		}
	}

	// Check for destructive commands
	destructive := []string{"rm ", "mv ", "cp -r", "> ", ">> "}
	for _, d := range destructive {
		if strings.HasPrefix(cmd, d) {
			return true
		}
	}

	return false
}

// GetWorkDir returns the working directory.
func (r *Runner) GetWorkDir() string {
	return r.config.WorkDir
}

// SanitizePath sanitizes a path for use in commands.
func SanitizePath(path string) string {
	// Prevent command injection
	path = strings.ReplaceAll(path, ";", "")
	path = strings.ReplaceAll(path, "&", "")
	path = strings.ReplaceAll(path, "|", "")
	path = strings.ReplaceAll(path, "`", "")
	path = strings.ReplaceAll(path, "$", "")
	return filepath.Clean(path)
}
