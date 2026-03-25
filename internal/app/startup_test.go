package app

import (
	"strings"
	"testing"

	"github.com/vigo999/ms-cli/integrations/skills"
	"github.com/vigo999/ms-cli/ui/model"
)

func TestHandleStartupControlInputAcceptsYes(t *testing.T) {
	app := &Application{
		EventCh: make(chan model.Event, 1),
	}
	prompt := &pendingStartupPrompt{decisionCh: make(chan bool, 1)}
	app.startupPrompt = prompt

	if !app.handleStartupControlInput("y") {
		t.Fatal("expected startup prompt input to be consumed")
	}

	select {
	case decision := <-prompt.decisionCh:
		if !decision {
			t.Fatal("expected yes input to approve the update")
		}
	default:
		t.Fatal("expected startup prompt decision to be delivered")
	}

	if app.currentStartupPrompt() != nil {
		t.Fatal("expected startup prompt to be cleared after approval")
	}

	select {
	case ev := <-app.EventCh:
		if ev.Type != model.ToolSkill {
			t.Fatalf("event type = %s, want %s", ev.Type, model.ToolSkill)
		}
		if ev.ToolName != "Skill sync" {
			t.Fatalf("tool name = %q, want %q", ev.ToolName, "Skill sync")
		}
		if !strings.Contains(ev.Summary, "updating shared skills repo") {
			t.Fatalf("unexpected event summary %q", ev.Summary)
		}
	default:
		t.Fatal("expected UI event for startup prompt approval")
	}
}

func TestHandleStartupControlInputAllowsExitCommand(t *testing.T) {
	app := &Application{
		EventCh: make(chan model.Event, 1),
	}
	prompt := &pendingStartupPrompt{decisionCh: make(chan bool, 1)}
	app.startupPrompt = prompt

	if app.handleStartupControlInput("/exit") {
		t.Fatal("expected /exit to bypass the startup prompt handler")
	}

	if app.currentStartupPrompt() == nil {
		t.Fatal("expected startup prompt to remain pending")
	}

	select {
	case ev := <-app.EventCh:
		t.Fatalf("unexpected event: %+v", ev)
	default:
	}
}

func TestSkillCatalogNamesIncludesSortedNames(t *testing.T) {
	got := skillCatalogNames([]skills.SkillSummary{
		{Name: "performance-agent"},
		{Name: "failure-agent"},
		{Name: "setup-agent"},
	})

	want := []string{"failure-agent", "performance-agent", "setup-agent"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("skillCatalogNames() = %v, want %v", got, want)
	}
}

func TestNormalizeStartupSummaryStripsPrefix(t *testing.T) {
	got := normalizeStartupSummary("skills sync: repo dir: /tmp/demo")
	want := "repo dir: /tmp/demo"
	if got != want {
		t.Fatalf("normalizeStartupSummary() = %q, want %q", got, want)
	}
}
