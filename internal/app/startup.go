package app

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/vigo999/ms-cli/agent/loop"
	"github.com/vigo999/ms-cli/integrations/skills"
	"github.com/vigo999/ms-cli/ui/model"
	"github.com/vigo999/ms-cli/ui/slash"
)

const bootReadyToken = "__boot_ready__"

type pendingStartupPrompt struct {
	decisionCh chan bool
}

type uiEventWriter struct {
	emit func(string)

	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *uiEventWriter) Write(p []byte) (int, error) {
	if w == nil {
		return len(p), nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := w.buf.Write(p); err != nil {
		return 0, err
	}

	for {
		data := w.buf.Bytes()
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimSpace(string(data[:idx]))
		w.buf.Next(idx + 1)
		if line != "" && w.emit != nil {
			w.emit(line)
		}
	}

	return len(p), nil
}

func buildSystemPrompt(summaries []skills.SkillSummary) string {
	systemPrompt := loop.DefaultSystemPrompt()
	if len(summaries) == 0 {
		return systemPrompt
	}
	return systemPrompt + "\n\n## Available Skills\n\n" +
		"Use the load_skill tool to load a skill when the user's task matches one:\n\n" +
		skills.FormatSummaries(summaries)
}

func registerSkillCommands(summaries []skills.SkillSummary) {
	for _, s := range summaries {
		slash.Register(slash.Command{
			Name:        "/" + s.Name,
			Description: s.Description,
			Usage:       "/" + s.Name + " [request...]",
		})
	}
}

func (a *Application) startDeferredStartup() {
	if a == nil {
		return
	}
	a.startupOnce.Do(func() {
		if strings.TrimSpace(a.skillsHomeDir) != "" {
			go a.syncSharedSkills()
		}
		go a.emitUpdateHint()
	})
}

func (a *Application) syncSharedSkills() {
	if a == nil || strings.TrimSpace(a.skillsHomeDir) == "" {
		return
	}

	logger := &uiEventWriter{emit: a.emitStartupMessage}
	repoSync := skills.NewRepoSync(skills.RepoSyncConfig{
		HomeDir:       a.skillsHomeDir,
		LogWriter:     logger,
		ConfirmUpdate: a.confirmSkillsUpdate,
	})

	if err := repoSync.Sync(); err != nil {
		a.emitToolError("skills", "sync shared skills repo: %v", err)
		return
	}

	a.refreshSkillCatalog()
}

func (a *Application) refreshSkillCatalog() {
	if a == nil || a.skillLoader == nil {
		return
	}

	summaries := a.skillLoader.List()
	registerSkillCommands(summaries)

	if a.ctxManager != nil {
		a.ctxManager.SetSystemPrompt(buildSystemPrompt(summaries))
	}
	if err := a.persistSessionSnapshot(); err != nil {
		a.emitToolError("session", "Failed to persist session snapshot: %v", err)
	}

	if a.EventCh == nil {
		return
	}

	names := skillCatalogNames(summaries)
	toolName := fmt.Sprintf("Skill ready: %d available", len(names))
	summary := strings.Join(names, ", ")

	a.EventCh <- model.Event{
		Type:     model.ToolSkill,
		ToolName: toolName,
		Summary:  summary,
	}
}

func (a *Application) emitStartupMessage(message string) {
	if a == nil || a.EventCh == nil || strings.TrimSpace(message) == "" {
		return
	}
	a.EventCh <- model.Event{
		Type:     model.ToolSkill,
		ToolName: "Skill sync",
		Summary:  normalizeStartupSummary(message),
	}
}

func (a *Application) confirmSkillsUpdate(localCommit, remoteCommit string) (bool, error) {
	prompt := &pendingStartupPrompt{decisionCh: make(chan bool, 1)}

	a.startupMu.Lock()
	a.startupPrompt = prompt
	a.startupMu.Unlock()

	a.emitStartupMessage(fmt.Sprintf(
		"update available: local %s -> remote %s. reply y or n.",
		displayStartupCommit(localCommit),
		displayStartupCommit(remoteCommit),
	))

	decision, ok := <-prompt.decisionCh
	if !ok {
		return false, io.EOF
	}
	return decision, nil
}

func (a *Application) handleStartupControlInput(input string) bool {
	prompt := a.currentStartupPrompt()
	if prompt == nil {
		return false
	}

	switch strings.ToLower(strings.TrimSpace(input)) {
	case "/exit":
		return false
	case "y", "yes", "是":
		a.resolveStartupPrompt(prompt, true)
		a.emitStartupMessage("skills sync: updating shared skills repo")
		return true
	case "n", "no", "否":
		a.resolveStartupPrompt(prompt, false)
		a.emitStartupMessage("skills sync: skipped update")
		return true
	default:
		a.emitStartupMessage("skills sync: reply y or n to continue the shared skills update check")
		return true
	}
}

func (a *Application) currentStartupPrompt() *pendingStartupPrompt {
	if a == nil {
		return nil
	}
	a.startupMu.Lock()
	defer a.startupMu.Unlock()
	return a.startupPrompt
}

func (a *Application) resolveStartupPrompt(prompt *pendingStartupPrompt, decision bool) {
	if a == nil || prompt == nil {
		return
	}

	a.startupMu.Lock()
	if a.startupPrompt == prompt {
		a.startupPrompt = nil
	}
	a.startupMu.Unlock()

	select {
	case prompt.decisionCh <- decision:
	default:
	}
	close(prompt.decisionCh)
}

func displayStartupCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if commit == "" {
		return "unknown"
	}
	if len(commit) <= 7 {
		return commit
	}
	return commit[:7]
}

func skillCatalogNames(summaries []skills.SkillSummary) []string {
	names := make([]string, 0, len(summaries))
	for _, s := range summaries {
		name := strings.TrimSpace(s.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func normalizeStartupSummary(message string) string {
	message = strings.TrimSpace(message)
	message = strings.TrimSpace(strings.TrimPrefix(message, "skills sync:"))
	return message
}
