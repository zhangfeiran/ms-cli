package skill

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	integrationskills "github.com/vigo999/ms-cli/integrations/skills"
)

func TestLoadToolWrapsSkillBodyWithoutFrontMatter(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "code-review", `---
name: code-review
description: Review code changes
---

# Review Skill

Check correctness first.`)

	tool := NewLoadTool(integrationskills.NewCatalog([]integrationskills.Source{
		{Name: "workdir", Root: root},
	}))

	params, err := json.Marshal(map[string]string{"name": "code-review"})
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("Execute returned tool error: %v", result.Error)
	}

	if strings.Contains(result.Content, "description: Review code changes") {
		t.Fatalf("tool result should not include YAML front matter: %q", result.Content)
	}
	if !strings.Contains(result.Content, `<loaded_skill name="code-review">`) {
		t.Fatalf("missing XML wrapper: %q", result.Content)
	}
	if result.Summary != "loaded skill: code-review (workdir)" {
		t.Fatalf("unexpected summary: %q", result.Summary)
	}
}

func writeSkill(t *testing.T, root, name, content string) {
	t.Helper()

	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}
