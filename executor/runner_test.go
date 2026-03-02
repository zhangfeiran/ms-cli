package executor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	results map[string]shell.Result
}

func (s *stubShell) Run(_ context.Context, command string) shell.Result {
	if out, ok := s.results[command]; ok {
		return out
	}
	return shell.Result{
		Output:     "unknown command\n",
		ReturnCode: 1,
	}
}
