package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const skillFileName = "SKILL.md"

// ErrSkillNotFound indicates that no source provided the requested skill.
var ErrSkillNotFound = errors.New("skill not found")

// Source identifies one skills directory in priority order.
type Source struct {
	Name string
	Root string
}

// SkillSummary is the compact prompt-time representation of a skill.
type SkillSummary struct {
	Name        string
	DisplayName string
	Description string
	Source      string
	Path        string
}

// LoadedSkill contains the full stripped body for one skill.
type LoadedSkill struct {
	SkillSummary
	Body string
}

// Catalog resolves skills across one or more sources.
type Catalog struct {
	sources []Source
}

type frontMatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// NewCatalog creates a catalog from explicit sources.
func NewCatalog(sources []Source) *Catalog {
	copied := make([]Source, 0, len(sources))
	for _, source := range sources {
		if strings.TrimSpace(source.Root) == "" {
			continue
		}
		copied = append(copied, source)
	}
	return &Catalog{sources: copied}
}

// DefaultCatalog creates a catalog using built-in, home, then workdir sources.
func DefaultCatalog(workDir string) *Catalog {
	return NewCatalog(DefaultSources(workDir))
}

// DefaultSources returns the default skills search path in ascending priority.
func DefaultSources(workDir string) []Source {
	sources := []Source{
		{
			Name: "builtin",
			Root: builtinSkillsDir(),
		},
	}

	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		sources = append(sources, Source{
			Name: "home",
			Root: filepath.Join(home, ".ms-cli", "skills"),
		})
	}

	if strings.TrimSpace(workDir) != "" {
		sources = append(sources, Source{
			Name: "workdir",
			Root: filepath.Join(workDir, ".ms-cli", "skills"),
		})
	}

	return sources
}

// List returns the effective skill summaries after applying source precedence.
func (c *Catalog) List() ([]SkillSummary, error) {
	merged := make(map[string]SkillSummary)
	var errs []error

	for _, source := range c.sources {
		summaries, err := listSource(source)
		if err != nil {
			errs = append(errs, err)
		}
		for _, summary := range summaries {
			merged[summary.Name] = summary
		}
	}

	result := make([]SkillSummary, 0, len(merged))
	for _, summary := range merged {
		result = append(result, summary)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, errors.Join(errs...)
}

// Names returns the effective skill names.
func (c *Catalog) Names() []string {
	summaries, _ := c.List()
	names := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		names = append(names, summary.Name)
	}
	return names
}

// Load resolves and loads one skill using descending source priority.
func (c *Catalog) Load(name string) (*LoadedSkill, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("skill name is required")
	}

	for i := len(c.sources) - 1; i >= 0; i-- {
		skill, err := loadFromSource(c.sources[i], name)
		if err == nil {
			return skill, nil
		}
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		return nil, err
	}

	return nil, fmt.Errorf("%w: %s", ErrSkillNotFound, name)
}

// FormatPromptSection renders the compact YAML skill list for the system prompt.
func FormatPromptSection(summaries []SkillSummary) string {
	if len(summaries) == 0 {
		return ""
	}

	items := make([]map[string]string, 0, len(summaries))
	for _, summary := range summaries {
		items = append(items, map[string]string{
			"name":        summary.Name,
			"description": summary.Description,
		})
	}

	data, err := yaml.Marshal(items)
	if err != nil {
		return ""
	}

	return "\n\nAvailable skills:\n```yaml\n" + strings.TrimSpace(string(data)) + "\n```"
}

// WrapToolResult wraps stripped skill instructions for tool-result context.
func WrapToolResult(name, body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return fmt.Sprintf("<loaded_skill name=\"%s\"></loaded_skill>", escapeAttr(name))
	}
	return fmt.Sprintf("<loaded_skill name=\"%s\">\n%s\n</loaded_skill>", escapeAttr(name), body)
}

func builtinSkillsDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".ms-cli", "skills"))
}

func listSource(source Source) ([]SkillSummary, error) {
	entries, err := os.ReadDir(source.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list %s skills: %w", source.Name, err)
	}

	var result []SkillSummary
	var errs []error

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skill, err := loadFromSource(source, entry.Name())
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			errs = append(errs, err)
			continue
		}

		result = append(result, skill.SkillSummary)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, errors.Join(errs...)
}

func loadFromSource(source Source, name string) (*LoadedSkill, error) {
	skillDir := filepath.Join(source.Root, name)
	info, err := os.Stat(skillDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("stat skill %s in %s: %w", name, source.Name, err)
	}
	if !info.IsDir() {
		return nil, os.ErrNotExist
	}

	filePath := filepath.Join(skillDir, skillFileName)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("read skill %s from %s: %w", name, source.Name, err)
	}

	meta, body, err := parseSkillContent(data)
	if err != nil {
		return nil, fmt.Errorf("parse skill %s from %s: %w", name, source.Name, err)
	}

	return &LoadedSkill{
		SkillSummary: SkillSummary{
			Name:        name,
			DisplayName: strings.TrimSpace(meta.Name),
			Description: strings.TrimSpace(meta.Description),
			Source:      source.Name,
			Path:        filePath,
		},
		Body: body,
	}, nil
}

func parseSkillContent(data []byte) (frontMatter, string, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	meta := frontMatter{}

	if !strings.HasPrefix(text, "---\n") {
		return meta, strings.TrimSpace(text), nil
	}

	lines := strings.Split(text, "\n")
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return meta, "", fmt.Errorf("unterminated yaml front matter")
	}

	header := strings.Join(lines[1:end], "\n")
	if err := yaml.Unmarshal([]byte(header), &meta); err != nil {
		return meta, "", fmt.Errorf("yaml front matter: %w", err)
	}

	body := strings.Join(lines[end+1:], "\n")
	return meta, strings.TrimSpace(body), nil
}

func escapeAttr(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"\"", "&quot;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(value)
}
