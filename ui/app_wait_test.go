package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vigo999/ms-cli/ui/model"
)

func TestEnterStartsThinkingWaitImmediately(t *testing.T) {
	userCh := make(chan string, 1)
	app := New(nil, userCh, "test", ".", "", "demo-model", 4096)
	app.bootActive = false

	next, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	app = next.(App)
	app.input.Model.SetValue("hello")

	next, _ = app.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	app = next.(App)

	if !app.state.IsThinking {
		t.Fatal("expected enter to start model wait immediately")
	}
	if got, want := app.state.WaitKind, model.WaitModel; got != want {
		t.Fatalf("wait kind = %v, want %v", got, want)
	}
	if view := app.View(); !strings.Contains(view, "Thinking... 00:0") {
		t.Fatalf("expected thinking timer in view, got:\n%s", view)
	}
}

func TestToolCallStartShowsPendingToolWait(t *testing.T) {
	app := New(nil, nil, "test", ".", "", "demo-model", 4096)
	app.bootActive = false

	next, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	app = next.(App)

	next, _ = app.handleEvent(model.Event{
		Type:     model.ToolCallStart,
		ToolName: "shell",
		Message:  "go test ./ui",
	})
	app = next.(App)

	if app.state.IsThinking {
		t.Fatal("expected tool wait to stop thinking state")
	}
	if got, want := app.state.WaitKind, model.WaitTool; got != want {
		t.Fatalf("wait kind = %v, want %v", got, want)
	}
	view := app.View()
	if !strings.Contains(view, "Shell($ go test ./ui)") {
		t.Fatalf("expected pending tool call line, got:\n%s", view)
	}
	if !strings.Contains(view, "running command... 00:0") {
		t.Fatalf("expected tool wait timer in view, got:\n%s", view)
	}
}

func TestToolWarningClearsWaitState(t *testing.T) {
	app := New(nil, nil, "test", ".", "", "demo-model", 4096)
	app.bootActive = false
	app.state = app.startWait(model.WaitTool)

	next, _ := app.handleEvent(model.Event{
		Type:     model.ToolWarning,
		ToolName: "Engine",
		Message:  "request timeout",
	})
	app = next.(App)

	if app.state.IsThinking {
		t.Fatal("expected warning to clear thinking state")
	}
	if got, want := app.state.WaitKind, model.WaitNone; got != want {
		t.Fatalf("wait kind = %v, want %v", got, want)
	}
	if got, want := app.state.Messages[len(app.state.Messages)-1].Display, model.DisplayWarning; got != want {
		t.Fatalf("last message display = %v, want %v", got, want)
	}
}
