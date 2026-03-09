package app

import (
	"context"

	"github.com/vigo999/ms-cli/agent/loop"
	"github.com/vigo999/ms-cli/agent/orchestrator"
)

// engineAdapter adapts loop.Engine to the orchestrator.Engine interface.
// This is the bridge between orchestrator-owned types and loop-owned types.
type engineAdapter struct {
	engine *loop.Engine
}

func newEngineAdapter(engine *loop.Engine) *engineAdapter {
	return &engineAdapter{engine: engine}
}

// Run converts orchestrator.RunRequest → loop.Task, calls the engine,
// and converts []loop.Event → []orchestrator.RunEvent.
func (a *engineAdapter) Run(ctx context.Context, req orchestrator.RunRequest) ([]orchestrator.RunEvent, error) {
	task := loop.Task{
		ID:          req.ID,
		Description: req.Description,
	}

	events, err := a.engine.RunWithContext(ctx, task)
	if err != nil {
		return nil, err
	}

	result := make([]orchestrator.RunEvent, 0, len(events))
	for _, ev := range events {
		result = append(result, orchestrator.RunEvent{
			Type:       ev.Type,
			Message:    ev.Message,
			ToolName:   ev.ToolName,
			Summary:    ev.Summary,
			CtxUsed:    ev.CtxUsed,
			CtxMax:     ev.CtxMax,
			TokensUsed: ev.TokensUsed,
			Timestamp:  ev.Timestamp,
		})
	}

	return result, nil
}
