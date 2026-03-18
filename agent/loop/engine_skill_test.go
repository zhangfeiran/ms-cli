package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/vigo999/ms-cli/integrations/llm"
	"github.com/vigo999/ms-cli/tools"
)

type skillLoadingProvider struct {
	callCount int
	lastReq   *llm.CompletionRequest
}

func (p *skillLoadingProvider) Name() string {
	return "skill-loader"
}

func (p *skillLoadingProvider) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.CompletionResponse, error) {
	p.callCount++
	if p.callCount == 1 {
		return &llm.CompletionResponse{
			ToolCalls: []llm.ToolCall{
				{
					ID:   "call_skill_1",
					Type: "function",
					Function: llm.ToolCallFunc{
						Name:      "load_skill",
						Arguments: json.RawMessage(`{"name":"code-review"}`),
					},
				},
			},
			FinishReason: llm.FinishToolCalls,
		}, nil
	}

	copied := *req
	copied.Messages = append([]llm.Message(nil), req.Messages...)
	p.lastReq = &copied

	return &llm.CompletionResponse{
		Content:      "done",
		FinishReason: llm.FinishStop,
	}, nil
}

func (p *skillLoadingProvider) CompleteStream(ctx context.Context, req *llm.CompletionRequest) (llm.StreamIterator, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *skillLoadingProvider) SupportsTools() bool {
	return true
}

func (p *skillLoadingProvider) AvailableModels() []llm.ModelInfo {
	return nil
}

type fakeLoadSkillTool struct{}

func (t *fakeLoadSkillTool) Name() string {
	return "load_skill"
}

func (t *fakeLoadSkillTool) Description() string {
	return "Load skill"
}

func (t *fakeLoadSkillTool) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Type: "object",
	}
}

func (t *fakeLoadSkillTool) Execute(ctx context.Context, params json.RawMessage) (*tools.Result, error) {
	_ = ctx
	_ = params
	return tools.StringResultWithSummary(
		`<loaded_skill name="code-review"># Review Skill</loaded_skill>`,
		"loaded skill: code-review (workdir)",
	), nil
}

func TestRunHandlesLoadSkillToolWithContextPayloadAndSummaryEvent(t *testing.T) {
	provider := &skillLoadingProvider{}
	registry := tools.NewRegistry()
	registry.MustRegister(&fakeLoadSkillTool{})

	engine := NewEngine(EngineConfig{
		MaxIterations: 3,
		MaxTokens:     8000,
	}, provider, registry)

	events, err := engine.Run(Task{
		ID:          "task-skill-load",
		Description: "review these changes",
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	var skillEvent Event
	foundSkillEvent := false
	for _, event := range events {
		if event.Type == EventToolSkillLoad {
			skillEvent = event
			foundSkillEvent = true
			break
		}
	}
	if !foundSkillEvent {
		t.Fatalf("expected %s event, got %#v", EventToolSkillLoad, events)
	}
	if skillEvent.Message != "loaded skill: code-review (workdir)" {
		t.Fatalf("unexpected event message: %q", skillEvent.Message)
	}
	if skillEvent.Summary != "loaded skill: code-review (workdir)" {
		t.Fatalf("unexpected event summary: %q", skillEvent.Summary)
	}

	if provider.lastReq == nil {
		t.Fatal("expected second completion request to be captured")
	}

	foundToolMessage := false
	for _, msg := range provider.lastReq.Messages {
		if msg.Role == "tool" && msg.ToolCallID == "call_skill_1" {
			foundToolMessage = true
			if msg.Content != `<loaded_skill name="code-review"># Review Skill</loaded_skill>` {
				t.Fatalf("unexpected tool message content: %q", msg.Content)
			}
		}
	}
	if !foundToolMessage {
		t.Fatalf("expected tool message in second request: %#v", provider.lastReq.Messages)
	}
}
