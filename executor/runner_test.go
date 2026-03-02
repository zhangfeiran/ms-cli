package executor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vigo999/ms-cli/agent/loop"
	"github.com/vigo999/ms-cli/tools/shell"
)

func TestRunCompletesOnSubmitToken(t *testing.T) {
	t.Setenv("MSCLI_AGENT_STEP_LIMIT", "4")
	trajPath := filepath.Join(t.TempDir(), "trajectory.json")
	t.Setenv("MSCLI_TRAJECTORY_PATH", trajPath)

	fakeModel := &stubLLM{
		replies: []ModelReply{
			{
				Content: "I will inspect first.",
				ToolCalls: []ToolCall{
					{ID: "call_1", Name: "bash", Arguments: `{"command":"pwd"}`},
				},
			},
			{
				Content: "Submitting result.",
				ToolCalls: []ToolCall{
					{ID: "call_2", Name: "bash", Arguments: `{"command":"submit"}`},
				},
			},
		},
	}
	fakeShell := &stubShell{
		results: map[string]shell.Result{
			"pwd": {
				Output:     "/repo\n",
				ReturnCode: 0,
			},
			"submit": {
				Output:     "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\nFixed successfully\n",
				ReturnCode: 0,
			},
		},
	}

	SetLLMClient(fakeModel)
	SetShellRunner(fakeShell)
	t.Cleanup(func() {
		SetLLMClient(nil)
		SetShellRunner(shell.Tool{})
	})

	events, err := runAndCollectEvents(loop.Task{Description: "fix it"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected events")
	}

	last := events[len(events)-1]
	if last.Type != eventAgentReply {
		t.Fatalf("unexpected last event type: %s", last.Type)
	}
	if last.Message != "Fixed successfully" {
		t.Fatalf("unexpected final message: %q", last.Message)
	}

	raw, err := os.ReadFile(trajPath)
	if err != nil {
		t.Fatalf("read trajectory: %v", err)
	}

	var traj Trajectory
	if err := json.Unmarshal(raw, &traj); err != nil {
		t.Fatalf("parse trajectory: %v", err)
	}
	if traj.ExitStatus != "submitted" {
		t.Fatalf("unexpected exit status: %s", traj.ExitStatus)
	}
	if traj.Submission != "Fixed successfully" {
		t.Fatalf("unexpected submission: %q", traj.Submission)
	}
	if len(traj.Steps) == 0 || len(traj.Steps[0].Commands) == 0 {
		t.Fatalf("expected commands in trajectory")
	}
}

func TestRunAddsFormatErrorWhenToolCallMissing(t *testing.T) {
	t.Setenv("MSCLI_AGENT_STEP_LIMIT", "4")
	t.Setenv("MSCLI_TRAJECTORY_PATH", filepath.Join(t.TempDir(), "trajectory.json"))

	fakeModel := &stubLLM{
		replies: []ModelReply{
			{
				Content:   "I forgot tool calls.",
				ToolCalls: nil,
			},
			{
				Content: "Retry with tool.",
				ToolCalls: []ToolCall{
					{ID: "call_2", Name: "bash", Arguments: `{"command":"submit"}`},
				},
			},
		},
	}
	fakeShell := &stubShell{
		results: map[string]shell.Result{
			"submit": {
				Output:     "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\ndone\n",
				ReturnCode: 0,
			},
		},
	}

	SetLLMClient(fakeModel)
	SetShellRunner(fakeShell)
	t.Cleanup(func() {
		SetLLMClient(nil)
		SetShellRunner(shell.Tool{})
	})

	events, err := runAndCollectEvents(loop.Task{Description: "fix it"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	foundToolError := false
	for _, ev := range events {
		if ev.Type == eventToolError {
			foundToolError = true
			break
		}
	}
	if !foundToolError {
		t.Fatalf("expected tool error event")
	}

	if len(fakeModel.calls) < 2 {
		t.Fatalf("expected at least 2 model calls, got %d", len(fakeModel.calls))
	}
	secondCall := fakeModel.calls[1]
	if len(secondCall) == 0 || secondCall[len(secondCall)-1].Role != "user" {
		t.Fatalf("unexpected second call history")
	}
	if secondCall[len(secondCall)-1].Content == "" {
		t.Fatalf("expected format error feedback in second call")
	}
}

func TestRunGeneratesToolCallIDAndAppendsObservationFallback(t *testing.T) {
	t.Setenv("MSCLI_AGENT_STEP_LIMIT", "4")
	t.Setenv("MSCLI_TRAJECTORY_PATH", filepath.Join(t.TempDir(), "trajectory.json"))
	t.Setenv("MSCLI_TEXT_OBSERVATION_FALLBACK", "true")

	fakeModel := &stubLLM{
		replies: []ModelReply{
			{
				Content: "Run pwd first.",
				ToolCalls: []ToolCall{
					{Name: "bash", Arguments: `{"command":"pwd"}`}, // no ID from model
				},
			},
			{
				Content: "Submit now.",
				ToolCalls: []ToolCall{
					{ID: "call_2", Name: "bash", Arguments: `{"command":"submit"}`},
				},
			},
		},
	}
	fakeShell := &stubShell{
		results: map[string]shell.Result{
			"pwd": {
				Output:     "/repo\n",
				ReturnCode: 0,
			},
			"submit": {
				Output:     "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\nok\n",
				ReturnCode: 0,
			},
		},
	}

	SetLLMClient(fakeModel)
	SetShellRunner(fakeShell)
	t.Cleanup(func() {
		SetLLMClient(nil)
		SetShellRunner(shell.Tool{})
	})

	_, err := runAndCollectEvents(loop.Task{Description: "fix it"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if len(fakeModel.calls) < 2 {
		t.Fatalf("expected at least 2 model calls, got %d", len(fakeModel.calls))
	}
	secondCall := fakeModel.calls[1]
	foundToolObs := false
	foundUserFallback := false
	for _, msg := range secondCall {
		if msg.Role == "tool" && msg.ToolCallID != "" && strings.Contains(msg.Content, `"returncode"`) && strings.Contains(msg.Content, "/repo") {
			foundToolObs = true
		}
		if msg.Role == "user" && strings.Contains(msg.Content, "Observation:") && strings.Contains(msg.Content, "/repo") {
			foundUserFallback = true
		}
	}
	if !foundToolObs {
		t.Fatalf("expected tool observation with generated tool_call_id in second model call")
	}
	if !foundUserFallback {
		t.Fatalf("expected user observation fallback in second model call")
	}
}

func TestRunObservationContainsStderr(t *testing.T) {
	t.Setenv("MSCLI_AGENT_STEP_LIMIT", "3")
	t.Setenv("MSCLI_TRAJECTORY_PATH", filepath.Join(t.TempDir(), "trajectory.json"))

	fakeModel := &stubLLM{
		replies: []ModelReply{
			{
				Content: "run",
				ToolCalls: []ToolCall{
					{ID: "call_1", Name: "bash", Arguments: `{"command":"warn"}`},
				},
			},
			{
				Content: "submit",
				ToolCalls: []ToolCall{
					{ID: "call_2", Name: "bash", Arguments: `{"command":"submit"}`},
				},
			},
		},
	}
	fakeShell := &stubShell{
		results: map[string]shell.Result{
			"warn": {
				Stdout:     "",
				Stderr:     "warning: risky\n",
				ReturnCode: 0,
			},
			"submit": {
				Output:     "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\nok\n",
				ReturnCode: 0,
			},
		},
	}

	SetLLMClient(fakeModel)
	SetShellRunner(fakeShell)
	t.Cleanup(func() {
		SetLLMClient(nil)
		SetShellRunner(shell.Tool{})
	})

	_, err := runAndCollectEvents(loop.Task{Description: "fix it"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(fakeModel.calls) < 2 {
		t.Fatalf("expected at least 2 model calls, got %d", len(fakeModel.calls))
	}

	secondCall := fakeModel.calls[1]
	foundStderr := false
	for _, msg := range secondCall {
		if strings.Contains(msg.Content, "warning: risky") {
			foundStderr = true
			break
		}
	}
	if !foundStderr {
		t.Fatalf("expected stderr text in observation messages")
	}
}

func TestRunTerminatesOnAssistantSubmitTokenWithoutToolCall(t *testing.T) {
	t.Setenv("MSCLI_AGENT_STEP_LIMIT", "4")
	trajPath := filepath.Join(t.TempDir(), "trajectory.json")
	t.Setenv("MSCLI_TRAJECTORY_PATH", trajPath)

	fakeModel := &stubLLM{
		replies: []ModelReply{
			{
				Content: "count first",
				ToolCalls: []ToolCall{
					{ID: "call_1", Name: "bash", Arguments: `{"command":"find . -name \"*.go\" | wc -l"}`},
				},
			},
			{
				Content:   "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\n当前路径及其子路径下一共有39个.go文件。",
				ToolCalls: nil,
			},
		},
	}
	fakeShell := &stubShell{
		results: map[string]shell.Result{
			`find . -name "*.go" | wc -l`: {
				Stdout:     "39\n",
				ReturnCode: 0,
			},
		},
	}

	SetLLMClient(fakeModel)
	SetShellRunner(fakeShell)
	t.Cleanup(func() {
		SetLLMClient(nil)
		SetShellRunner(shell.Tool{})
	})

	events, err := runAndCollectEvents(loop.Task{Description: "当前路径及其子路径下一共有多少个.go文件"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected events")
	}
	last := events[len(events)-1]
	if last.Type != eventAgentReply {
		t.Fatalf("expected final agent reply, got %s", last.Type)
	}
	if !strings.Contains(last.Message, "39") {
		t.Fatalf("expected final answer contain 39, got %q", last.Message)
	}

	raw, err := os.ReadFile(trajPath)
	if err != nil {
		t.Fatalf("read trajectory: %v", err)
	}
	var traj Trajectory
	if err := json.Unmarshal(raw, &traj); err != nil {
		t.Fatalf("parse trajectory: %v", err)
	}
	if traj.ExitStatus != "submitted_by_assistant" {
		t.Fatalf("unexpected exit status: %s", traj.ExitStatus)
	}
	if !strings.Contains(traj.Submission, "39") {
		t.Fatalf("unexpected submission: %q", traj.Submission)
	}
}

func TestRunTerminatesOnAssistantFinalAnswerWithoutToolCallAfterTool(t *testing.T) {
	t.Setenv("MSCLI_AGENT_STEP_LIMIT", "6")
	trajPath := filepath.Join(t.TempDir(), "trajectory.json")
	t.Setenv("MSCLI_TRAJECTORY_PATH", trajPath)

	fakeModel := &stubLLM{
		replies: []ModelReply{
			{
				Content: "count first",
				ToolCalls: []ToolCall{
					{ID: "call_1", Name: "bash", Arguments: `{"command":"find . -name \"*.go\" | wc -l"}`},
				},
			},
			{
				Content:   "Therefore, there are 39 .go files in the current path and its subdirectories.",
				ToolCalls: nil,
			},
		},
	}
	fakeShell := &stubShell{
		results: map[string]shell.Result{
			`find . -name "*.go" | wc -l`: {
				Stdout:     "39\n",
				ReturnCode: 0,
			},
		},
	}

	SetLLMClient(fakeModel)
	SetShellRunner(fakeShell)
	t.Cleanup(func() {
		SetLLMClient(nil)
		SetShellRunner(shell.Tool{})
	})

	events, err := runAndCollectEvents(loop.Task{Description: "当前路径及其子路径下一共有多少个.go文件"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected events")
	}
	toolErrors := 0
	for _, ev := range events {
		if ev.Type == eventToolError {
			toolErrors++
		}
	}
	if toolErrors != 0 {
		t.Fatalf("expected no tool_error when assistant already returned final answer, got %d", toolErrors)
	}

	raw, err := os.ReadFile(trajPath)
	if err != nil {
		t.Fatalf("read trajectory: %v", err)
	}
	var traj Trajectory
	if err := json.Unmarshal(raw, &traj); err != nil {
		t.Fatalf("parse trajectory: %v", err)
	}
	if traj.ExitStatus != "submitted_by_assistant" {
		t.Fatalf("unexpected exit status: %s", traj.ExitStatus)
	}
	if len(traj.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(traj.Steps))
	}
	if !strings.Contains(traj.Submission, "39") {
		t.Fatalf("unexpected submission: %q", traj.Submission)
	}
	if len(traj.Messages) == 0 || traj.Messages[len(traj.Messages)-1].Role != "exit" {
		t.Fatalf("expected trailing exit message, got %+v", traj.Messages[len(traj.Messages)-1])
	}
}

func TestRunShortCircuitsEchoSubmitCommandLikeMini(t *testing.T) {
	t.Setenv("MSCLI_AGENT_STEP_LIMIT", "4")
	trajPath := filepath.Join(t.TempDir(), "trajectory.json")
	t.Setenv("MSCLI_TRAJECTORY_PATH", trajPath)

	findCmd := `find . -name "*.go" | wc -l`
	fakeModel := &stubLLM{
		replies: []ModelReply{
			{
				Content: "count first",
				ToolCalls: []ToolCall{
					{ID: "call_1", Name: "bash", Arguments: `{"command":"find . -name \"*.go\" | wc -l"}`},
				},
			},
			{
				Content: "The result is 39, now submit.",
				ToolCalls: []ToolCall{
					{ID: "call_2", Name: "bash", Arguments: `{"command":"echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"}`},
				},
			},
		},
	}
	fakeShell := &stubShell{
		results: map[string]shell.Result{
			findCmd: {
				Stdout:     "39\n",
				ReturnCode: 0,
			},
		},
	}

	SetLLMClient(fakeModel)
	SetShellRunner(fakeShell)
	t.Cleanup(func() {
		SetLLMClient(nil)
		SetShellRunner(shell.Tool{})
	})

	_, err := runAndCollectEvents(loop.Task{Description: "当前路径及其子路径下一共有多少个.go文件"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if len(fakeShell.commands) != 1 || fakeShell.commands[0] != findCmd {
		t.Fatalf("submit command should not execute in shell, got commands=%v", fakeShell.commands)
	}

	raw, err := os.ReadFile(trajPath)
	if err != nil {
		t.Fatalf("read trajectory: %v", err)
	}
	var traj Trajectory
	if err := json.Unmarshal(raw, &traj); err != nil {
		t.Fatalf("parse trajectory: %v", err)
	}
	if traj.ExitStatus != "submitted" {
		t.Fatalf("unexpected exit status: %s", traj.ExitStatus)
	}
	if len(traj.Messages) == 0 || traj.Messages[len(traj.Messages)-1].Role != "exit" {
		t.Fatalf("expected trailing exit message, got %+v", traj.Messages[len(traj.Messages)-1])
	}
	if len(traj.Steps) < 2 || len(traj.Steps[1].Commands) == 0 {
		t.Fatalf("expected submit step command record in trajectory")
	}
	submitCmd := traj.Steps[1].Commands[0]
	if submitCmd.ReturnCode != -1 {
		t.Fatalf("expected submit returncode -1, got %d", submitCmd.ReturnCode)
	}
	if submitCmd.ExceptionInfo != "action was not executed" {
		t.Fatalf("unexpected submit exception_info: %q", submitCmd.ExceptionInfo)
	}
}

func TestRenderPromptForDebugReadableFormat(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "user prompt"},
		{
			Role:    "assistant",
			Content: "assistant reply",
			ToolCalls: []ToolCall{
				{ID: "call_1", Name: "bash", Arguments: `{"command":"pwd"}`},
			},
		},
	}
	tools := []ToolSpec{{Name: "bash"}}

	out := renderPromptForDebug(1, msgs, tools)
	if !strings.Contains(out, "Step 1 Prompt:") {
		t.Fatalf("missing step header: %q", out)
	}
	for _, section := range []string{"System:\n", "User:\n", "Assistant:\n", "Tool Calls:\n", "Available Tools:\n"} {
		if !strings.Contains(out, section) {
			t.Fatalf("missing section %q in output: %q", section, out)
		}
	}
	if strings.Contains(out, `"messages"`) || strings.Contains(out, `"tools"`) {
		t.Fatalf("should not render JSON payload: %q", out)
	}
}

func TestEmitShellOutputMiniStyle(t *testing.T) {
	events := make([]loop.Event, 0, 16)
	emitShellOutput(func(ev loop.Event) {
		events = append(events, ev)
	}, shell.Result{
		Output:        "39\n",
		ReturnCode:    0,
		ExceptionInfo: "action was not executed",
	})

	got := make([]string, 0, len(events))
	for _, ev := range events {
		if ev.Type == eventCmdOutput {
			got = append(got, ev.Message)
		}
	}

	expect := []string{
		"<returncode>",
		"0",
		"<output>",
		"39",
		"<exception_info>",
		"action was not executed",
	}
	if len(got) != len(expect) {
		t.Fatalf("unexpected cmd output event count: got=%d want=%d events=%v", len(got), len(expect), got)
	}
	for i := range expect {
		if got[i] != expect[i] {
			t.Fatalf("unexpected event[%d]: got=%q want=%q all=%v", i, got[i], expect[i], got)
		}
	}
}

func runAndCollectEvents(task loop.Task) ([]loop.Event, error) {
	events := make([]loop.Event, 0, 32)
	err := Run(task, func(ev loop.Event) {
		events = append(events, ev)
	})
	return events, err
}

type stubLLM struct {
	replies []ModelReply
	calls   [][]Message
}

func (s *stubLLM) Chat(_ context.Context, messages []Message, _ []ToolSpec) (ModelReply, error) {
	copied := make([]Message, len(messages))
	for i := range messages {
		copied[i] = Message{
			Role:       messages[i].Role,
			Content:    messages[i].Content,
			ToolCallID: messages[i].ToolCallID,
			ToolCalls:  append([]ToolCall(nil), messages[i].ToolCalls...),
		}
	}
	s.calls = append(s.calls, copied)

	if len(s.replies) == 0 {
		return ModelReply{}, nil
	}
	reply := s.replies[0]
	s.replies = s.replies[1:]
	return reply, nil
}

type stubShell struct {
	results  map[string]shell.Result
	commands []string
}

func (s *stubShell) Run(_ context.Context, command string) shell.Result {
	s.commands = append(s.commands, command)
	if out, ok := s.results[command]; ok {
		return out
	}
	return shell.Result{
		Output:     "unknown command\n",
		ReturnCode: 1,
	}
}
