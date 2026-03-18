package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentctx "github.com/vigo999/ms-cli/agent/context"
	"github.com/vigo999/ms-cli/configs"
	"github.com/vigo999/ms-cli/integrations/llm"
	integrationskills "github.com/vigo999/ms-cli/integrations/skills"
	"github.com/vigo999/ms-cli/ui/model"
)

func TestHandleSkillCommandInjectsToolCallAndResult(t *testing.T) {
	app := newSkillTestApp(t, "code-review", `---
name: code-review
description: Review code
---

# Review Skill

Focus on correctness.`)

	app.handleCommand("/code-review")

	messages := app.ctxManager.GetNonSystemMessages()
	if len(messages) != 2 {
		t.Fatalf("expected 2 injected messages, got %d", len(messages))
	}
	if messages[0].Role != "assistant" {
		t.Fatalf("expected assistant tool-call message, got %q", messages[0].Role)
	}
	if len(messages[0].ToolCalls) != 1 || messages[0].ToolCalls[0].Function.Name != "load_skill" {
		t.Fatalf("expected load_skill tool call, got %#v", messages[0].ToolCalls)
	}
	if messages[1].Role != "tool" {
		t.Fatalf("expected tool result message, got %q", messages[1].Role)
	}
	if !strings.Contains(messages[1].Content, `<loaded_skill name="code-review">`) {
		t.Fatalf("expected wrapped tool result, got %q", messages[1].Content)
	}
	if strings.Contains(messages[1].Content, "description: Review code") {
		t.Fatalf("tool result should not include YAML front matter: %q", messages[1].Content)
	}
}

func TestBuiltInCommandKeepsPriorityOverSameNamedSkill(t *testing.T) {
	app := newSkillTestApp(t, "help", `---
name: help
description: Not the built-in help command
---

# Fake Help`)

	app.handleCommand("/help")

	if got := len(app.ctxManager.GetNonSystemMessages()); got != 0 {
		t.Fatalf("expected no skill injection for built-in command, got %d messages", got)
	}

	select {
	case ev := <-app.EventCh:
		if ev.Type != model.AgentReply || !strings.Contains(ev.Message, "Available commands") {
			t.Fatalf("unexpected help event: %#v", ev)
		}
	default:
		t.Fatal("expected help response event")
	}
}

func TestCmdClearClearsBackendContext(t *testing.T) {
	app := newSkillTestApp(t, "code-review", `# Review Skill`)

	if err := app.ctxManager.AddMessage(llm.NewUserMessage("hello")); err != nil {
		t.Fatalf("AddMessage failed: %v", err)
	}
	if err := app.ctxManager.AddToolResult("call_1", "tool output"); err != nil {
		t.Fatalf("AddToolResult failed: %v", err)
	}

	app.cmdClear()

	if got := len(app.ctxManager.GetNonSystemMessages()); got != 0 {
		t.Fatalf("expected cleared backend context, got %d messages", got)
	}
	if app.ctxManager.GetSystemPrompt() == nil {
		t.Fatal("expected system prompt to be preserved after clear")
	}
}

func newSkillTestApp(t *testing.T, name, content string) *Application {
	t.Helper()

	skillRoot := t.TempDir()
	writeSkillFixture(t, skillRoot, name, content)

	catalog := integrationskills.NewCatalog([]integrationskills.Source{
		{Name: "workdir", Root: skillRoot},
	})
	summaries, err := catalog.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	ctxManager := agentctx.NewManager(agentctx.ManagerConfig{
		MaxTokens:     8000,
		ReserveTokens: 4000,
	})
	ctxManager.SetSystemPrompt("system")

	cfg := configs.DefaultConfig()
	app := &Application{
		EventCh:        make(chan model.Event, 16),
		Config:         cfg,
		toolRegistry:   initTools(cfg, t.TempDir(), catalog),
		ctxManager:     ctxManager,
		skillCatalog:   catalog,
		skillSummaries: summaries,
	}
	return app
}

func writeSkillFixture(t *testing.T, root, name, content string) {
	t.Helper()

	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}
