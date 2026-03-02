package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vigo999/ms-cli/agent/loop"
	"github.com/vigo999/ms-cli/tools/shell"
)

const (
	defaultStepLimit          = 12
	defaultLLMTimeoutSeconds  = 90
	defaultCmdTimeoutSeconds  = 30
	maxObservationOutputChars = 10000
	submitToken               = "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"
	defaultTrajectoryPath     = "trace/last-trajectory.json"
	trajectoryFormatVersion   = "ms-cli-minimal-agent-v1"
)

const (
	eventAgentThinking = "agent_thinking"
	eventAgentReply    = "agent_reply"
	eventCmdStarted    = "cmd_started"
	eventCmdOutput     = "cmd_output"
	eventCmdFinished   = "cmd_finished"
	eventToolError     = "tool_error"
	eventDebugPrompt   = "debug_prompt"
)

var systemPrompt = "You are a coding agent. Use bash tool calls to inspect/edit/test the codebase."

// Message is one conversation message exchanged with the LLM.
type Message struct {
	Role       string
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
}

// ToolCall is one tool invocation from the assistant.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// ToolSpec declares one tool available to the model.
type ToolSpec struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ModelReply is one assistant output.
type ModelReply struct {
	Content   string
	ToolCalls []ToolCall
}

// LLMClient is the interface executor needs.
type LLMClient interface {
	Chat(ctx context.Context, messages []Message, tools []ToolSpec) (ModelReply, error)
}

// ShellRunner executes shell commands.
type ShellRunner interface {
	Run(ctx context.Context, command string) shell.Result
}

// Trajectory stores one full run, inspired by mini-SWE-agent trajectory.
type Trajectory struct {
	TrajectoryFormat string              `json:"trajectory_format"`
	Task             string              `json:"task"`
	StartedAt        time.Time           `json:"started_at"`
	FinishedAt       time.Time           `json:"finished_at"`
	ExitStatus       string              `json:"exit_status"`
	Error            string              `json:"error,omitempty"`
	Submission       string              `json:"submission,omitempty"`
	Steps            []TrajectoryStep    `json:"steps"`
	Messages         []TrajectoryMessage `json:"messages"`
}

// TrajectoryStep stores one model step.
type TrajectoryStep struct {
	Step       int                  `json:"step"`
	StartedAt  time.Time            `json:"started_at"`
	FinishedAt time.Time            `json:"finished_at"`
	Assistant  string               `json:"assistant,omitempty"`
	ToolCalls  []TrajectoryToolCall `json:"tool_calls,omitempty"`
	Commands   []TrajectoryCommand  `json:"commands,omitempty"`
	Error      string               `json:"error,omitempty"`
}

// TrajectoryMessage stores one conversation message.
type TrajectoryMessage struct {
	Role       string                      `json:"role"`
	Content    string                      `json:"content,omitempty"`
	ToolCallID string                      `json:"tool_call_id,omitempty"`
	ToolCalls  []TrajectoryMessageToolCall `json:"tool_calls,omitempty"`
}

// TrajectoryMessageToolCall is one tool call inside an assistant message.
type TrajectoryMessageToolCall struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

// TrajectoryToolCall stores normalized tool call info in a step.
type TrajectoryToolCall struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

// TrajectoryCommand stores one executed shell command and output.
type TrajectoryCommand struct {
	ToolCallID    string    `json:"tool_call_id,omitempty"`
	Command       string    `json:"command"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	ReturnCode    int       `json:"returncode"`
	Output        string    `json:"output,omitempty"`
	ExceptionInfo string    `json:"exception_info,omitempty"`
}

var (
	llmClient   LLMClient
	shellRunner ShellRunner = shell.Tool{}
)

// SetLLMClient injects an LLM client.
func SetLLMClient(client LLMClient) {
	llmClient = client
}

// SetSystemPrompt overrides the default system prompt for the agent loop.
func SetSystemPrompt(prompt string) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return
	}
	systemPrompt = prompt
}

// SetShellRunner injects a shell runner (mainly for tests).
func SetShellRunner(runner ShellRunner) {
	if runner == nil {
		return
	}
	shellRunner = runner
}

// Run executes a minimal multi-step SWE-agent loop and streams events.
func Run(task loop.Task, emit func(loop.Event)) (runErr error) {
	taskDesc := strings.TrimSpace(task.Description)
	if taskDesc == "" {
		return fmt.Errorf("task description is empty")
	}

	traj := &Trajectory{
		TrajectoryFormat: trajectoryFormatVersion,
		Task:             taskDesc,
		StartedAt:        time.Now().UTC(),
		ExitStatus:       "running",
		Steps:            make([]TrajectoryStep, 0, 16),
		Messages:         make([]TrajectoryMessage, 0, 64),
	}
	defer func() {
		traj.FinishedAt = time.Now().UTC()
		if runErr != nil {
			traj.Error = runErr.Error()
			if traj.ExitStatus == "running" {
				traj.ExitStatus = "error"
			}
		}
		if err := saveTrajectory(traj); err != nil && runErr == nil {
			runErr = fmt.Errorf("save trajectory: %w", err)
		}
	}()

	if llmClient == nil {
		emitEvent(emit, loop.Event{Type: eventAgentReply, Message: "Executed: " + taskDesc})
		traj.ExitStatus = "no_llm_client"
		return nil
	}

	messages := []Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: buildTaskPrompt(taskDesc),
		},
	}
	for _, msg := range messages {
		appendTrajectoryMessage(traj, msg)
	}

	tools := []ToolSpec{bashToolSpec()}

	for step := 1; step <= envInt("MSCLI_AGENT_STEP_LIMIT", defaultStepLimit); step++ {
		stepStartedAt := time.Now().UTC()
		traj.Steps = append(traj.Steps, TrajectoryStep{
			Step:      step,
			StartedAt: stepStartedAt,
		})
		stepIndex := len(traj.Steps) - 1

		emitEvent(emit, loop.Event{Type: eventAgentThinking})
		if envBool("MSCLI_DEBUG_PROMPT", true) {
			emitEvent(emit, loop.Event{
				Type:    eventDebugPrompt,
				Message: renderPromptForDebug(step, messages, tools),
			})
		}

		llmCtx, cancel := context.WithTimeout(context.Background(), time.Duration(envInt("MSCLI_LLM_TIMEOUT_SECONDS", defaultLLMTimeoutSeconds))*time.Second)
		reply, err := llmClient.Chat(llmCtx, messages, tools)
		cancel()
		if err != nil {
			traj.Steps[stepIndex].FinishedAt = time.Now().UTC()
			traj.Steps[stepIndex].Error = err.Error()
			traj.ExitStatus = "llm_error"
			return fmt.Errorf("llm chat failed at step %d: %w", step, err)
		}

		reply.ToolCalls = normalizeToolCalls(step, reply.ToolCalls)

		content := strings.TrimSpace(reply.Content)
		if content != "" {
			emitEvent(emit, loop.Event{Type: eventAgentReply, Message: content})
		}

		traj.Steps[stepIndex].Assistant = content
		traj.Steps[stepIndex].ToolCalls = copyToolCalls(reply.ToolCalls)

		assistantMessage := Message{
			Role:      "assistant",
			Content:   reply.Content,
			ToolCalls: reply.ToolCalls,
		}
		messages = append(messages, assistantMessage)
		appendTrajectoryMessage(traj, assistantMessage)

		if len(reply.ToolCalls) == 0 {
			errMsg := "No tool calls found. Every response must include at least one bash tool call."
			emitEvent(emit, loop.Event{
				Type:     eventToolError,
				ToolName: "Agent",
				Message:  errMsg,
			})

			feedback := Message{
				Role:    "user",
				Content: buildFormatError(errMsg),
			}
			messages = append(messages, feedback)
			appendTrajectoryMessage(traj, feedback)

			traj.Steps[stepIndex].Error = errMsg
			traj.Steps[stepIndex].FinishedAt = time.Now().UTC()
			continue
		}

		parseFailed := false
		for _, call := range reply.ToolCalls {
			command, err := parseCommandFromToolCall(call)
			if err != nil {
				parseFailed = true
				emitEvent(emit, loop.Event{
					Type:     eventToolError,
					ToolName: "bash",
					Message:  err.Error(),
				})

				feedback := Message{
					Role:    "user",
					Content: buildFormatError(err.Error()),
				}
				messages = append(messages, feedback)
				appendTrajectoryMessage(traj, feedback)

				traj.Steps[stepIndex].Error = err.Error()
				break
			}

			cmdStartedAt := time.Now().UTC()
			emitEvent(emit, loop.Event{Type: eventCmdStarted, Message: command})

			cmdCtx, cmdCancel := context.WithTimeout(context.Background(), time.Duration(envInt("MSCLI_COMMAND_TIMEOUT_SECONDS", defaultCmdTimeoutSeconds))*time.Second)
			result := shellRunner.Run(cmdCtx, command)
			cmdCancel()

			for _, line := range splitOutputLines(result.Output) {
				emitEvent(emit, loop.Event{Type: eventCmdOutput, Message: line})
			}
			if result.ExceptionInfo != "" {
				emitEvent(emit, loop.Event{Type: eventCmdOutput, Message: result.ExceptionInfo})
			}
			emitEvent(emit, loop.Event{Type: eventCmdFinished})

			cmdFinishedAt := time.Now().UTC()
			traj.Steps[stepIndex].Commands = append(traj.Steps[stepIndex].Commands, TrajectoryCommand{
				ToolCallID:    call.ID,
				Command:       command,
				StartedAt:     cmdStartedAt,
				FinishedAt:    cmdFinishedAt,
				ReturnCode:    result.ReturnCode,
				Output:        result.Output,
				ExceptionInfo: result.ExceptionInfo,
			})

			toolMessage := Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    formatObservation(result),
			}
			messages = append(messages, toolMessage)
			appendTrajectoryMessage(traj, toolMessage)
			if envBool("MSCLI_TEXT_OBSERVATION_FALLBACK", true) {
				fallback := Message{
					Role: "user",
					Content: "Observation:\n" + toolMessage.Content + "\n\n" +
						"Continue with the next action.",
				}
				messages = append(messages, fallback)
				appendTrajectoryMessage(traj, fallback)
			}

			if submitted, submission := detectSubmission(result); submitted {
				final := strings.TrimSpace(submission)
				if final == "" {
					final = "Task completed."
				}
				emitEvent(emit, loop.Event{Type: eventAgentReply, Message: final})

				traj.Submission = final
				traj.ExitStatus = "submitted"
				traj.Steps[stepIndex].FinishedAt = time.Now().UTC()
				return nil
			}
		}

		traj.Steps[stepIndex].FinishedAt = time.Now().UTC()
		if parseFailed {
			continue
		}
	}

	traj.ExitStatus = "step_limit_exceeded"
	return fmt.Errorf("agent step limit exceeded (%d)", envInt("MSCLI_AGENT_STEP_LIMIT", defaultStepLimit))
}

func emitEvent(emit func(loop.Event), ev loop.Event) {
	if emit == nil {
		return
	}
	emit(ev)
}

func saveTrajectory(traj *Trajectory) error {
	path := strings.TrimSpace(os.Getenv("MSCLI_TRAJECTORY_PATH"))
	if path == "" {
		path = defaultTrajectoryPath
	}
	if strings.EqualFold(path, "off") {
		return nil
	}

	data, err := json.MarshalIndent(traj, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal trajectory: %w", err)
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create trajectory dir %q: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write trajectory %q: %w", path, err)
	}
	return nil
}

func appendTrajectoryMessage(traj *Trajectory, msg Message) {
	tm := TrajectoryMessage{
		Role:       msg.Role,
		Content:    msg.Content,
		ToolCallID: msg.ToolCallID,
	}
	if len(msg.ToolCalls) > 0 {
		tm.ToolCalls = make([]TrajectoryMessageToolCall, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			tm.ToolCalls = append(tm.ToolCalls, TrajectoryMessageToolCall{
				ID:        tc.ID,
				Name:      tc.Name,
				Arguments: tc.Arguments,
			})
		}
	}
	traj.Messages = append(traj.Messages, tm)
}

func copyToolCalls(calls []ToolCall) []TrajectoryToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]TrajectoryToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, TrajectoryToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
	}
	return out
}

func bashToolSpec() ToolSpec {
	return ToolSpec{
		Name:        "bash",
		Description: "Execute a bash command",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The bash command to execute",
				},
			},
			"required": []string{"command"},
		},
	}
}

func buildTaskPrompt(task string) string {
	return "Please solve this task: " + task + "\n\n" +
		"Each response MUST include at least one bash tool call.\n" +
		"To finish, run exactly:\n" +
		"echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\n" +
		"Then print the final answer in following lines of stdout."
}

func buildFormatError(err string) string {
	return "Tool call error:\n\n" + err + "\n\n" +
		"Every response must call tool 'bash' with JSON args: {\"command\": \"...\"}."
}

func parseCommandFromToolCall(call ToolCall) (string, error) {
	if strings.TrimSpace(call.Name) != "bash" {
		return "", fmt.Errorf("unknown tool %q", call.Name)
	}

	raw := strings.TrimSpace(call.Arguments)
	if raw == "" {
		return "", fmt.Errorf("missing tool call arguments")
	}

	var payload struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		var direct string
		if err2 := json.Unmarshal([]byte(raw), &direct); err2 == nil {
			payload.Command = direct
		} else {
			return "", fmt.Errorf("invalid tool call arguments: %v", err)
		}
	}

	command := strings.TrimSpace(payload.Command)
	if command == "" {
		return "", fmt.Errorf("missing 'command' argument in bash tool call")
	}
	return command, nil
}

func formatObservation(result shell.Result) string {
	output := result.Output
	msg := map[string]any{
		"returncode": result.ReturnCode,
	}

	if len(output) <= maxObservationOutputChars {
		msg["output"] = output
	} else {
		msg["output_head"] = output[:maxObservationOutputChars/2]
		msg["output_tail"] = output[len(output)-maxObservationOutputChars/2:]
		msg["elided_chars"] = len(output) - maxObservationOutputChars
		msg["warning"] = "Output too long."
	}
	if result.ExceptionInfo != "" {
		msg["exception_info"] = result.ExceptionInfo
	}

	raw, err := json.Marshal(msg)
	if err != nil {
		return fmt.Sprintf(`{"returncode":%d,"output":"%s"}`, result.ReturnCode, escapeJSON(output))
	}
	return string(raw)
}

func escapeJSON(s string) string {
	raw, _ := json.Marshal(s)
	if len(raw) >= 2 {
		return string(raw[1 : len(raw)-1])
	}
	return s
}

func splitOutputLines(output string) []string {
	trimmed := strings.TrimRight(output, "\n")
	if trimmed == "" {
		return nil
	}

	lines := strings.Split(trimmed, "\n")
	const maxLines = 200
	if len(lines) > maxLines {
		tail := lines[len(lines)-maxLines:]
		return append([]string{fmt.Sprintf("... output truncated (%d lines omitted) ...", len(lines)-maxLines)}, tail...)
	}
	return lines
}

func detectSubmission(result shell.Result) (bool, string) {
	if result.ReturnCode != 0 {
		return false, ""
	}
	trimmed := strings.TrimLeft(result.Output, " \t\r\n")
	if trimmed == "" {
		return false, ""
	}

	lines := strings.Split(trimmed, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != submitToken {
		return false, ""
	}
	if len(lines) == 1 {
		return true, ""
	}
	return true, strings.Join(lines[1:], "\n")
}

func normalizeToolCalls(step int, calls []ToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(calls))
	for i, call := range calls {
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = fmt.Sprintf("call_%d_%d", step, i+1)
		}
		out = append(out, ToolCall{
			ID:        id,
			Name:      strings.TrimSpace(call.Name),
			Arguments: strings.TrimSpace(call.Arguments),
		})
	}
	return out
}

func renderPromptForDebug(step int, messages []Message, tools []ToolSpec) string {
	payload := map[string]any{
		"step":     step,
		"messages": messages,
		"tools":    tools,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Sprintf("step=%d\nprompt_marshal_error=%v", step, err)
	}
	return string(raw)
}

func envBool(key string, defaultValue bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return defaultValue
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func envInt(key string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return defaultValue
	}
	return v
}
