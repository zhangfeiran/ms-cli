package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/vigo999/ms-cli/agent/loop"
	"github.com/vigo999/ms-cli/executor"
	"github.com/vigo999/ms-cli/integrations/llm/openai"
	"github.com/vigo999/ms-cli/ui/model"
)

// Bootstrap wires top-level dependencies.
func Bootstrap(demo bool) (*Application, error) {
	if err := wireExecutor(demo); err != nil {
		return nil, err
	}
	loop.SetExecutorRun(executor.Run)

	workDir, err := os.Getwd()
	if err != nil {
		workDir = "."
	}
	workDir, _ = filepath.Abs(workDir)

	return &Application{
		Engine:  loop.NewEngine(),
		EventCh: make(chan model.Event, 64),
		Demo:    demo,
		WorkDir: workDir,
		RepoURL: "github.com/vigo999/ms-cli",
	}, nil
}

func wireExecutor(demo bool) error {
	executor.SetLLMClient(nil)
	if demo {
		return nil
	}

	cfg, err := loadRuntimeConfig(defaultConfigPath)
	if err != nil {
		return err
	}
	if cfg.Model.Provider != "openai" {
		return fmt.Errorf("unsupported model provider %q", cfg.Model.Provider)
	}
	executor.SetSystemPrompt(cfg.Model.SystemPrompt)

	client, err := openai.NewClient(openai.Config{
		BaseURL:      cfg.Model.Endpoint,
		APIKey:       cfg.Model.APIKey,
		Model:        cfg.Model.Name,
		SystemPrompt: cfg.Model.SystemPrompt,
		HTTPClient:   &http.Client{Timeout: cfg.Model.Timeout()},
	})
	if err != nil {
		return fmt.Errorf("init llm client: %w", err)
	}
	executor.SetLLMClient(openAIExecutorAdapter{client: client})
	return nil
}

type openAIExecutorAdapter struct {
	client *openai.Client
}

func (a openAIExecutorAdapter) Chat(ctx context.Context, messages []executor.Message, tools []executor.ToolSpec) (executor.ModelReply, error) {
	reqMsgs := make([]openai.Message, 0, len(messages))
	for _, msg := range messages {
		calls := make([]openai.ToolCall, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			calls = append(calls, openai.ToolCall{
				ID:        tc.ID,
				Name:      tc.Name,
				Arguments: tc.Arguments,
			})
		}
		reqMsgs = append(reqMsgs, openai.Message{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
			ToolCalls:  calls,
		})
	}

	reqTools := make([]openai.ToolSpec, 0, len(tools))
	for _, t := range tools {
		reqTools = append(reqTools, openai.ToolSpec{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}

	resp, err := a.client.Chat(ctx, reqMsgs, reqTools)
	if err != nil {
		return executor.ModelReply{}, err
	}

	outCalls := make([]executor.ToolCall, 0, len(resp.ToolCalls))
	for _, tc := range resp.ToolCalls {
		outCalls = append(outCalls, executor.ToolCall{
			ID:        tc.ID,
			Name:      tc.Name,
			Arguments: tc.Arguments,
		})
	}

	return executor.ModelReply{
		Content:   resp.Content,
		ToolCalls: outCalls,
	}, nil
}
