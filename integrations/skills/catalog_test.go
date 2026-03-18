package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogListUsesSourcePrecedence(t *testing.T) {
	builtin := t.TempDir()
	home := t.TempDir()
	workdir := t.TempDir()

	writeSkillFile(t, builtin, "code-review", `---
name: code-review
description: builtin review
---

# Builtin`)
	writeSkillFile(t, home, "code-review", `---
name: code-review
description: home review
---

# Home`)
	writeSkillFile(t, workdir, "code-review", `---
name: code-review
description: workdir review
---

# Workdir`)
	writeSkillFile(t, home, "pdf", `---
name: pdf
description: pdf helper
---

# PDF`)

	catalog := NewCatalog([]Source{
		{Name: "builtin", Root: builtin},
		{Name: "home", Root: home},
		{Name: "workdir", Root: workdir},
	})

	summaries, err := catalog.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}

	var codeReview SkillSummary
	for _, summary := range summaries {
		if summary.Name == "code-review" {
			codeReview = summary
			break
		}
	}

	if codeReview.Description != "workdir review" {
		t.Fatalf("expected highest-priority description, got %q", codeReview.Description)
	}
	if codeReview.Source != "workdir" {
		t.Fatalf("expected highest-priority source, got %q", codeReview.Source)
	}
}

func TestCatalogLoadStripsFrontMatter(t *testing.T) {
	root := t.TempDir()
	writeSkillFile(t, root, "pdf", `---
name: pdf
description: Process PDF files
---

# PDF Processing Skill

Use pdftotext first.`)

	catalog := NewCatalog([]Source{{Name: "builtin", Root: root}})
	skill, err := catalog.Load("pdf")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if skill.Description != "Process PDF files" {
		t.Fatalf("unexpected description: %q", skill.Description)
	}
	if strings.Contains(skill.Body, "description: Process PDF files") {
		t.Fatalf("front matter should be stripped from body: %q", skill.Body)
	}
	if !strings.Contains(skill.Body, "# PDF Processing Skill") {
		t.Fatalf("body missing markdown content: %q", skill.Body)
	}
}

func TestCatalogLoadKeepsBodyWithoutFrontMatter(t *testing.T) {
	root := t.TempDir()
	writeSkillFile(t, root, "plain", "# Plain Skill\n\nNo YAML here.")

	catalog := NewCatalog([]Source{{Name: "builtin", Root: root}})
	skill, err := catalog.Load("plain")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if skill.Description != "" {
		t.Fatalf("expected empty description, got %q", skill.Description)
	}
	if skill.Body != "# Plain Skill\n\nNo YAML here." {
		t.Fatalf("unexpected body: %q", skill.Body)
	}
}

func TestWrapToolResult(t *testing.T) {
	wrapped := WrapToolResult("code-review", "# Review Skill")
	if !strings.Contains(wrapped, `<loaded_skill name="code-review">`) {
		t.Fatalf("missing wrapper start: %q", wrapped)
	}
	if !strings.Contains(wrapped, "# Review Skill") {
		t.Fatalf("missing wrapped body: %q", wrapped)
	}
	if !strings.Contains(wrapped, "</loaded_skill>") {
		t.Fatalf("missing wrapper end: %q", wrapped)
	}
}

func writeSkillFile(t *testing.T, root, name, content string) {
	t.Helper()

	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, skillFileName), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}
