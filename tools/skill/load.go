package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/vigo999/ms-cli/integrations/llm"
	integrationskills "github.com/vigo999/ms-cli/integrations/skills"
	"github.com/vigo999/ms-cli/tools"
)

// LoadTool loads a named skill into LLM context.
type LoadTool struct {
	catalog *integrationskills.Catalog
}

// NewLoadTool creates a new load_skill tool.
func NewLoadTool(catalog *integrationskills.Catalog) *LoadTool {
	return &LoadTool{catalog: catalog}
}

// Name returns the tool name.
func (t *LoadTool) Name() string {
	return "load_skill"
}

// Description returns the tool description.
func (t *LoadTool) Description() string {
	return "Load the full instructions for one available skill into context. Use this when the user's task clearly matches a listed skill."
}

// Schema returns the tool parameter schema.
func (t *LoadTool) Schema() llm.ToolSchema {
	names := t.catalog.Names()
	sort.Strings(names)

	return llm.ToolSchema{
		Type: "object",
		Properties: map[string]llm.Property{
			"name": {
				Type:        "string",
				Description: "The canonical skill name to load",
				Enum:        names,
			},
		},
		Required: []string{"name"},
	}
}

type loadParams struct {
	Name string `json:"name"`
}

// Execute loads the requested skill and wraps it for tool-result context.
func (t *LoadTool) Execute(ctx context.Context, params json.RawMessage) (*tools.Result, error) {
	_ = ctx

	var p loadParams
	if err := tools.ParseParams(params, &p); err != nil {
		return tools.ErrorResult(err), nil
	}

	name := strings.TrimSpace(p.Name)
	if name == "" {
		return tools.ErrorResultf("name is required"), nil
	}

	skill, err := t.catalog.Load(name)
	if err != nil {
		return tools.ErrorResult(err), nil
	}

	content := integrationskills.WrapToolResult(skill.Name, skill.Body)
	summary := fmt.Sprintf("loaded skill: %s (%s)", skill.Name, skill.Source)
	return tools.StringResultWithSummary(content, summary), nil
}
