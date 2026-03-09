// Package orchestrator dispatches tasks to the appropriate execution path
// based on the selected run mode (standard or plan).
package orchestrator

import (
	"context"
	"fmt"

	"github.com/vigo999/ms-cli/agent/planner"
)

// Engine is the interface the orchestrator uses to run tasks.
// Implemented by an adapter over agent/loop in internal/app/wire.go.
type Engine interface {
	Run(ctx context.Context, req RunRequest) ([]RunEvent, error)
}

// WorkflowRunner executes a sequence of plan steps.
// Implemented by workflow/engine (to be built).
type WorkflowRunner interface {
	Execute(ctx context.Context, steps []planner.Step) ([]StepResult, error)
}

// StepResult is the outcome of a single step execution.
type StepResult struct {
	Index   int
	Success bool
	Output  string
	Error   error
}

// Config holds orchestrator settings.
type Config struct {
	Mode           RunMode
	AvailableTools []string
}

// Orchestrator coordinates planner, workflow, and loop based on mode.
type Orchestrator struct {
	config   Config
	engine   Engine
	planner  *planner.Planner
	workflow WorkflowRunner
	callback PlanCallback
}

// New creates an Orchestrator.
func New(cfg Config, engine Engine, p *planner.Planner, wf WorkflowRunner) *Orchestrator {
	return &Orchestrator{
		config:   cfg,
		engine:   engine,
		planner:  p,
		workflow:  wf,
		callback: NoOpCallback{},
	}
}

// SetCallback sets the plan lifecycle callback.
func (o *Orchestrator) SetCallback(cb PlanCallback) {
	if cb == nil {
		o.callback = NoOpCallback{}
		return
	}
	o.callback = cb
}

// SetMode changes the run mode.
func (o *Orchestrator) SetMode(mode RunMode) {
	o.config.Mode = mode
}

// CurrentMode returns the current run mode.
func (o *Orchestrator) CurrentMode() RunMode {
	return o.config.Mode
}

// Run executes a task using the configured mode.
func (o *Orchestrator) Run(ctx context.Context, req RunRequest) ([]RunEvent, error) {
	switch o.config.Mode {
	case ModePlan:
		return o.runPlan(ctx, req)
	default:
		return o.engine.Run(ctx, req)
	}
}

// runPlan generates a plan then executes it step by step.
func (o *Orchestrator) runPlan(ctx context.Context, req RunRequest) ([]RunEvent, error) {
	var events []RunEvent

	// 1. Generate plan
	events = append(events, NewRunEvent(EventAgentThinking, "Generating plan..."))

	steps, err := o.planner.Plan(ctx, req.Description, o.config.AvailableTools)
	if err != nil {
		events = append(events, NewRunEvent(EventTaskFailed, fmt.Sprintf("Plan generation failed: %v", err)))
		return events, fmt.Errorf("generate plan: %w", err)
	}

	events = append(events, NewRunEvent(EventLLMResponse, fmt.Sprintf("Plan created with %d steps", len(steps))))

	// 2. Notify callback
	if err := o.callback.OnPlanCreated(steps); err != nil {
		events = append(events, NewRunEvent(EventTaskFailed, fmt.Sprintf("Plan rejected: %v", err)))
		return events, err
	}

	if err := o.callback.OnPlanApproved(steps); err != nil {
		return events, err
	}

	// 3. Execute via workflow runner
	if o.workflow == nil {
		return o.runPlanViaLoop(ctx, req, steps, events)
	}

	results, err := o.workflow.Execute(ctx, steps)
	if err != nil {
		events = append(events, NewRunEvent(EventTaskFailed, fmt.Sprintf("Plan execution failed: %v", err)))
		return events, fmt.Errorf("execute plan: %w", err)
	}

	// 4. Summarize results
	for _, r := range results {
		if r.Success {
			events = append(events, NewRunEvent(EventAgentReply, r.Output))
		} else {
			events = append(events, NewRunEvent(EventToolError, r.Error.Error()))
		}
	}

	events = append(events, NewRunEvent(EventTaskCompleted, "Plan completed"))
	return events, nil
}

// runPlanViaLoop falls back to running each step through the engine.
func (o *Orchestrator) runPlanViaLoop(ctx context.Context, parent RunRequest, steps []planner.Step, events []RunEvent) ([]RunEvent, error) {
	for i, step := range steps {
		if err := ctx.Err(); err != nil {
			events = append(events, NewRunEvent(EventTaskFailed, "Cancelled"))
			return events, err
		}

		if err := o.callback.OnStepStarted(step, i); err != nil {
			return events, err
		}

		stepReq := RunRequest{
			ID:          fmt.Sprintf("%s-step-%d", parent.ID, i),
			Description: step.Description,
		}

		stepEvents, err := o.engine.Run(ctx, stepReq)
		events = append(events, stepEvents...)

		result := ""
		if err != nil {
			result = err.Error()
		} else {
			result = "completed"
		}

		if cbErr := o.callback.OnStepCompleted(step, i, result); cbErr != nil {
			return events, cbErr
		}

		if err != nil {
			events = append(events, NewRunEvent(EventTaskFailed, fmt.Sprintf("Step %d failed: %v", i+1, err)))
			return events, fmt.Errorf("step %d: %w", i+1, err)
		}
	}

	events = append(events, NewRunEvent(EventTaskCompleted, "Plan completed"))
	return events, nil
}
