package loop

import (
	"context"
	"fmt"
	"time"

	ctxmanager "github.com/vigo999/ms-cli/agent/context"
	"github.com/vigo999/ms-cli/integrations/llm"
	"github.com/vigo999/ms-cli/tools"
)

// EngineConfig holds engine configuration.
type EngineConfig struct {
	MaxIterations  int
	MaxTokens      int
	Temperature    float32
	TimeoutPerTurn time.Duration
	SystemPrompt   string
}

// Engine drives task execution and emits events.
type Engine struct {
	config     EngineConfig
	provider   llm.Provider
	tools      *tools.Registry
	ctxManager *ctxmanager.Manager
	permission PermissionService
}

// NewEngine creates a new engine.
func NewEngine(cfg EngineConfig, provider llm.Provider, tools *tools.Registry) *Engine {
	// MaxIterations = 0 means no limit
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.7
	}
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = defaultSystemPrompt()
	}

	engine := &Engine{
		config:   cfg,
		provider: provider,
		tools:    tools,
	}

	// Initialize context manager if not set
	engine.ctxManager = ctxmanager.NewManager(ctxmanager.ManagerConfig{
		MaxTokens:     cfg.MaxTokens,
		ReserveTokens: 4000,
	})
	engine.ctxManager.SetSystemPrompt(cfg.SystemPrompt)

	// Default permission service
	engine.permission = NewNoOpPermissionService()

	return engine
}

// SetContextManager sets the context manager.
func (e *Engine) SetContextManager(cm *ctxmanager.Manager) {
	e.ctxManager = cm
}

// SetPermissionService sets the permission service.
func (e *Engine) SetPermissionService(ps PermissionService) {
	e.permission = ps
}

// Run executes a task and returns events.
func (e *Engine) Run(task Task) ([]Event, error) {
	ctx := context.Background()
	return e.RunWithContext(ctx, task)
}

// RunWithContext executes a task with context.
func (e *Engine) RunWithContext(ctx context.Context, task Task) ([]Event, error) {
	exec := &executor{
		engine:     e,
		task:       task,
		events:     make([]Event, 0),
		startTime:  time.Now(),
	}
	return exec.run(ctx)
}

// executor manages execution of a single task.
type executor struct {
	engine    *Engine
	task      Task
	events    []Event
	iterCount int
	startTime time.Time
	totalUsage llm.Usage
}

// run executes the ReAct loop.
func (ex *executor) run(ctx context.Context) ([]Event, error) {
	// Add initial user message
	ex.engine.ctxManager.AddMessage(llm.NewUserMessage(ex.task.Description))

	ex.addEvent(NewEvent(EventTaskStarted, fmt.Sprintf("Task: %s", ex.task.Description)))
	ex.addEvent(NewEvent(EventAgentThinking, ""))

	for ex.engine.config.MaxIterations == 0 || ex.iterCount < ex.engine.config.MaxIterations {
		ex.iterCount++

		// Check context cancellation
		if err := ctx.Err(); err != nil {
			ex.addEvent(NewEvent(EventTaskFailed, fmt.Sprintf("Context cancelled: %v", err)))
			return ex.events, err
		}

		// Get messages for LLM
		messages := ex.engine.ctxManager.GetMessages()
		tools := ex.engine.tools.ToLLMTools()

		// Call LLM
		ctx, cancel := context.WithTimeout(ctx, ex.engine.config.TimeoutPerTurn)
		resp, err := ex.engine.provider.Complete(ctx, &llm.CompletionRequest{
			Model:       "", // Use provider default
			Messages:    messages,
			Tools:       tools,
			Temperature: ex.engine.config.Temperature,
		})
		cancel()

		if err != nil {
			ex.addEvent(NewEvent(EventTaskFailed, fmt.Sprintf("LLM error: %v", err)))
			return ex.events, fmt.Errorf("LLM completion: %w", err)
		}

		// Track usage
		ex.totalUsage.PromptTokens += resp.Usage.PromptTokens
		ex.totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		ex.totalUsage.TotalTokens += resp.Usage.TotalTokens

		// Handle response
		continueLoop, err := ex.handleResponse(ctx, resp)
		if err != nil {
			ex.addEvent(NewEvent(EventTaskFailed, fmt.Sprintf("Handle response error: %v", err)))
			return ex.events, err
		}

		if !continueLoop {
			break
		}
	}

	// Check if stopped due to max iterations (only if limit is set)
	if ex.engine.config.MaxIterations > 0 && ex.iterCount >= ex.engine.config.MaxIterations {
		ex.addEvent(NewEvent(EventTaskFailed, "Task exceeded maximum iterations. The AI may be stuck in a loop or the task is too complex. Try breaking it into smaller steps or being more specific about what you want."))
	} else {
		ex.addEvent(NewEvent(EventTaskCompleted, "Task completed successfully"))
	}

	return ex.events, nil
}

// handleResponse processes the LLM response.
func (ex *executor) handleResponse(ctx context.Context, resp *llm.CompletionResponse) (bool, error) {
	// Add assistant message to context
	assistantMsg := llm.Message{
		Role:      "assistant",
		Content:   resp.Content,
		ToolCalls: resp.ToolCalls,
	}
	ex.engine.ctxManager.AddMessage(assistantMsg)

	// Handle tool calls
	if len(resp.ToolCalls) > 0 {
		// Execute tools
		for _, tc := range resp.ToolCalls {
			if err := ex.executeToolCall(ctx, tc); err != nil {
				return false, err
			}
		}
		return true, nil // Continue loop
	}

	// Final response
	if resp.Content != "" {
		ex.addEvent(NewEvent(EventAgentReply, resp.Content))
	}

	return false, nil // End loop
}

// executeToolCall executes a single tool call.
func (ex *executor) executeToolCall(ctx context.Context, tc llm.ToolCall) error {
	toolName := tc.Function.Name

	// Find tool
	tool, ok := ex.engine.tools.Get(toolName)
	if !ok {
		errMsg := fmt.Sprintf("Tool not found: %s", toolName)
		ex.addEvent(NewEvent(EventToolError, errMsg))
		ex.engine.ctxManager.AddToolResult(tc.ID, errMsg)
		return nil
	}

	// Check permission
	action := string(tc.Function.Arguments)
	granted, err := ex.engine.permission.Request(ctx, toolName, action, "")
	if err != nil {
		return err
	}
	if !granted {
		errMsg := fmt.Sprintf("Permission denied for tool: %s", toolName)
		ex.addEvent(NewEvent(EventToolError, errMsg))
		ex.engine.ctxManager.AddToolResult(tc.ID, errMsg)
		return nil
	}

	// Add event
	ex.addEvent(NewEvent(EventToolStarted, fmt.Sprintf("Using tool: %s", toolName)))

	// Execute tool
	result, err := tool.Execute(ctx, tc.Function.Arguments)
	if err != nil {
		errMsg := fmt.Sprintf("Tool execution error: %v", err)
		ex.addEvent(NewEvent(EventToolError, errMsg))
		ex.engine.ctxManager.AddToolResult(tc.ID, errMsg)
		return nil
	}

	// Handle error result
	if result.Error != nil {
		errMsg := result.Error.Error()
		ex.addEvent(NewEvent(EventToolError, fmt.Sprintf("Tool %s failed: %s", toolName, errMsg)))
		ex.engine.ctxManager.AddToolResult(tc.ID, errMsg)
		return nil
	}

	// Add tool event based on tool type
	ex.addToolEvent(toolName, result)

	// Add tool result to context
	ex.engine.ctxManager.AddToolResult(tc.ID, result.Content)

	return nil
}

// addToolEvent adds an event based on tool type.
func (ex *executor) addToolEvent(toolName string, result *tools.Result) {
	eventType := EventToolStarted
	switch toolName {
	case "read":
		eventType = EventToolRead
	case "grep":
		eventType = EventToolGrep
	case "glob":
		eventType = EventToolGlob
	case "edit":
		eventType = EventToolEdit
	case "write":
		eventType = EventToolWrite
	case "shell":
		eventType = EventCmdStarted
	}

	ev := NewEvent(eventType, result.Content)
	ev.ToolName = toolName
	ev.Summary = result.Summary
	ex.addEvent(ev)
}

// addEvent adds an event to the list.
func (ex *executor) addEvent(ev Event) {
	// Update token usage
	usage := ex.engine.ctxManager.TokenUsage()
	ev.CtxUsed = usage.Current
	ev.TokensUsed = ex.totalUsage.TotalTokens

	ex.events = append(ex.events, ev)
}

// defaultSystemPrompt returns the default system prompt.
func defaultSystemPrompt() string {
	return `You are an AI assistant that helps users with software development tasks.

You have access to the following tools:
- read: Read file contents
- write: Create or overwrite files
- edit: Edit files by replacing text
- grep: Search for patterns in files
- glob: Find files matching patterns
- shell: Execute shell commands

Guidelines:
1. Use tools to gather information before making changes
2. Always read files before editing them
3. Make minimal, focused changes
4. Use grep and glob to explore the codebase
5. Run tests with shell to verify changes

IMPORTANT: When you have gathered enough information to answer the user's question, you MUST provide your final answer directly WITHOUT using any more tools. Do not keep calling tools indefinitely - provide a clear, concise response once you have the information needed.

When making edits, ensure the old_string matches exactly (including whitespace and newlines).`
}

// SetExecutorRun sets the executor run function (for backward compatibility).
var executorRun = func(task Task) string {
	return "Executed: " + task.Description
}

// SetExecutorRun sets the executor run function.
func SetExecutorRun(run func(task Task) string) {
	if run != nil {
		executorRun = run
	}
}
