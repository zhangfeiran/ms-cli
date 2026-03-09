package orchestrator

import (
	"context"
	"testing"

	"github.com/vigo999/ms-cli/agent/planner"
	"github.com/vigo999/ms-cli/integrations/llm"
)

// mockEngine records calls and returns canned events.
type mockEngine struct {
	calls  []RunRequest
	events []RunEvent
	err    error
}

func (m *mockEngine) Run(_ context.Context, req RunRequest) ([]RunEvent, error) {
	m.calls = append(m.calls, req)
	if m.err != nil {
		return nil, m.err
	}
	return m.events, nil
}

// mockProvider returns a fixed response for planner.
type mockProvider struct {
	content string
}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) Complete(_ context.Context, _ *llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{Content: m.content}, nil
}
func (m *mockProvider) CompleteStream(_ context.Context, _ *llm.CompletionRequest) (llm.StreamIterator, error) {
	return nil, nil
}
func (m *mockProvider) SupportsTools() bool            { return false }
func (m *mockProvider) AvailableModels() []llm.ModelInfo { return nil }

func TestRun_StandardMode(t *testing.T) {
	engine := &mockEngine{
		events: []RunEvent{NewRunEvent(EventAgentReply, "done")},
	}

	o := New(Config{Mode: ModeStandard}, engine, nil, nil)

	req := RunRequest{ID: "1", Description: "hello"}
	events, err := o.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(engine.calls) != 1 {
		t.Fatalf("expected 1 engine call, got %d", len(engine.calls))
	}
	if engine.calls[0].Description != "hello" {
		t.Errorf("expected request 'hello', got %q", engine.calls[0].Description)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestRun_PlanMode_ViaLoop(t *testing.T) {
	engine := &mockEngine{
		events: []RunEvent{NewRunEvent(EventAgentReply, "step done")},
	}

	provider := &mockProvider{
		content: `[{"description":"Read file","tool":"read"},{"description":"Fix bug","tool":"edit"}]`,
	}
	p := planner.New(provider, planner.DefaultConfig())

	o := New(Config{Mode: ModePlan, AvailableTools: []string{"read", "edit"}}, engine, p, nil)

	req := RunRequest{ID: "t1", Description: "fix the bug"}
	events, err := o.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(engine.calls) != 2 {
		t.Fatalf("expected 2 engine calls, got %d", len(engine.calls))
	}
	if engine.calls[0].Description != "Read file" {
		t.Errorf("step 0: expected 'Read file', got %q", engine.calls[0].Description)
	}
	if engine.calls[1].Description != "Fix bug" {
		t.Errorf("step 1: expected 'Fix bug', got %q", engine.calls[1].Description)
	}

	hasCompleted := false
	for _, ev := range events {
		if ev.Type == EventTaskCompleted {
			hasCompleted = true
		}
	}
	if !hasCompleted {
		t.Error("expected TaskCompleted event")
	}
}

func TestParseRunMode(t *testing.T) {
	tests := []struct {
		input string
		want  RunMode
	}{
		{"standard", ModeStandard},
		{"plan", ModePlan},
		{"planning", ModePlan},
		{"", ModeStandard},
		{"unknown", ModeStandard},
	}

	for _, tt := range tests {
		got := ParseRunMode(tt.input)
		if got != tt.want {
			t.Errorf("ParseRunMode(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// trackingCallback records callback invocations.
type trackingCallback struct {
	created   int
	approved  int
	started   []int
	completed []int
}

func (c *trackingCallback) OnPlanCreated([]planner.Step) error               { c.created++; return nil }
func (c *trackingCallback) OnPlanApproved([]planner.Step) error              { c.approved++; return nil }
func (c *trackingCallback) OnStepStarted(_ planner.Step, i int) error        { c.started = append(c.started, i); return nil }
func (c *trackingCallback) OnStepCompleted(_ planner.Step, i int, _ string) error { c.completed = append(c.completed, i); return nil }

func TestPlanMode_Callbacks(t *testing.T) {
	engine := &mockEngine{
		events: []RunEvent{NewRunEvent(EventAgentReply, "ok")},
	}
	provider := &mockProvider{
		content: `[{"description":"step one"},{"description":"step two"}]`,
	}
	p := planner.New(provider, planner.DefaultConfig())
	cb := &trackingCallback{}

	o := New(Config{Mode: ModePlan}, engine, p, nil)
	o.SetCallback(cb)

	_, err := o.Run(context.Background(), RunRequest{ID: "t1", Description: "do stuff"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cb.created != 1 {
		t.Errorf("expected 1 OnPlanCreated, got %d", cb.created)
	}
	if cb.approved != 1 {
		t.Errorf("expected 1 OnPlanApproved, got %d", cb.approved)
	}
	if len(cb.started) != 2 {
		t.Errorf("expected 2 OnStepStarted, got %d", len(cb.started))
	}
	if len(cb.completed) != 2 {
		t.Errorf("expected 2 OnStepCompleted, got %d", len(cb.completed))
	}
}
