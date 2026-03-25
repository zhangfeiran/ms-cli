package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vigo999/ms-cli/internal/bugs"
	issuepkg "github.com/vigo999/ms-cli/internal/issues"
	projectpkg "github.com/vigo999/ms-cli/internal/project"
	"github.com/vigo999/ms-cli/ui/model"
)

type credentials struct {
	ServerURL string `json:"server_url"`
	Token     string `json:"token"`
	User      string `json:"user"`
	Role      string `json:"role"`
}

type serverProfile struct {
	User   string `json:"user"`
	Role   string `json:"role"`
	APIKey string `json:"api_key,omitempty"`
}

type serverProfileErrorKind int

const (
	serverProfileErrorRequestBuild serverProfileErrorKind = iota + 1
	serverProfileErrorTransport
	serverProfileErrorBadStatus
	serverProfileErrorDecode
)

type serverProfileError struct {
	kind       serverProfileErrorKind
	statusCode int
	body       string
	err        error
}

var newServerProfileHTTPClient = func() *http.Client {
	return &http.Client{Timeout: 5 * time.Second}
}

func serverProfileEndpoint(serverURL string) string {
	return strings.TrimRight(serverURL, "/") + "/me"
}

func runtimeAPIKeyMissingMessage(serverURL string) string {
	return fmt.Sprintf("server runtime api key unavailable: %s returned no api_key; check server env MSCLI_API_KEY", serverProfileEndpoint(serverURL))
}

func (e *serverProfileError) Error() string {
	if e == nil {
		return ""
	}
	if e.err != nil {
		return e.err.Error()
	}
	if strings.TrimSpace(e.body) != "" {
		return e.body
	}
	return fmt.Sprintf("server profile request failed with status %d", e.statusCode)
}

func (e *serverProfileError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func credentialsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ms-cli", "credentials.json")
}

func loadCredentials() (*credentials, error) {
	data, err := os.ReadFile(credentialsPath())
	if err != nil {
		return nil, err
	}
	var cred credentials
	if err := json.Unmarshal(data, &cred); err != nil {
		return nil, err
	}
	return &cred, nil
}

func saveCredentials(cred *credentials) error {
	dir := filepath.Dir(credentialsPath())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(credentialsPath(), data, 0o600)
}

func fetchServerProfile(serverURL, token string) (*serverProfile, error) {
	client := newServerProfileHTTPClient()
	req, err := http.NewRequest("GET", serverProfileEndpoint(serverURL), nil)
	if err != nil {
		return nil, &serverProfileError{kind: serverProfileErrorRequestBuild, err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, &serverProfileError{kind: serverProfileErrorTransport, err: err}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &serverProfileError{kind: serverProfileErrorDecode, err: err}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &serverProfileError{
			kind:       serverProfileErrorBadStatus,
			statusCode: resp.StatusCode,
			body:       strings.TrimSpace(string(body)),
		}
	}

	var profile serverProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		return nil, &serverProfileError{kind: serverProfileErrorDecode, err: err}
	}
	return &profile, nil
}

func (a *Application) applyRuntimeAPIKey(apiKey string) error {
	if a == nil || a.Config == nil {
		return nil
	}
	if strings.TrimSpace(a.Config.Model.Key) != "" {
		return nil
	}

	trimmedKey := strings.TrimSpace(apiKey)
	if trimmedKey == "" {
		return nil
	}
	return a.SetProvider("", "", trimmedKey)
}

func (a *Application) tryEnableRuntimeAPIKeyFromLogin() error {
	if a == nil || a.Config == nil || a.llmReady {
		return nil
	}
	if strings.TrimSpace(a.Config.Model.Key) != "" {
		return nil
	}

	cred := a.loginCred
	if cred == nil {
		var err error
		cred, err = loadCredentials()
		if err != nil {
			return nil
		}
		a.loginCred = cred
	}

	profile, err := fetchServerProfile(cred.ServerURL, cred.Token)
	if err != nil {
		return fmt.Errorf("server runtime api key fetch failed via %s: %w", serverProfileEndpoint(cred.ServerURL), err)
	}
	if strings.TrimSpace(profile.User) != "" {
		a.issueUser = profile.User
	}
	if strings.TrimSpace(profile.Role) != "" {
		a.issueRole = profile.Role
	}
	if strings.TrimSpace(profile.APIKey) == "" {
		return fmt.Errorf("%s", runtimeAPIKeyMissingMessage(cred.ServerURL))
	}
	return a.applyRuntimeAPIKey(profile.APIKey)
}

func (a *Application) cmdLogin(args []string) {
	if len(args) == 0 {
		a.EventCh <- model.Event{
			Type:    model.AgentReply,
			Message: "Usage: /login <token>",
		}
		return
	}
	serverURL := strings.TrimRight(a.Config.Issues.ServerURL, "/")
	if serverURL == "" {
		a.EventCh <- model.Event{
			Type:    model.AgentReply,
			Message: "server URL not set. Run: export MSCLI_SERVER_URL=http://<host>:9473",
		}
		return
	}
	token := args[0]

	profile, err := fetchServerProfile(serverURL, token)
	if err != nil {
		var profileErr *serverProfileError
		if errors.As(err, &profileErr) {
			switch profileErr.kind {
			case serverProfileErrorRequestBuild:
				a.EventCh <- model.Event{Type: model.AgentReply, Message: fmt.Sprintf("login failed: %v", profileErr.err)}
				return
			case serverProfileErrorTransport:
				a.EventCh <- model.Event{Type: model.AgentReply, Message: fmt.Sprintf("login failed: cannot reach server: %v", profileErr.err)}
				return
			case serverProfileErrorBadStatus:
				a.EventCh <- model.Event{Type: model.AgentReply, Message: fmt.Sprintf("login failed: %s", profileErr.body)}
				return
			case serverProfileErrorDecode:
				a.EventCh <- model.Event{Type: model.AgentReply, Message: fmt.Sprintf("login failed: invalid response: %v", profileErr.err)}
				return
			}
		}
		a.EventCh <- model.Event{Type: model.AgentReply, Message: fmt.Sprintf("login failed: %v", err)}
		return
	}

	cred := &credentials{
		ServerURL: serverURL,
		Token:     token,
		User:      profile.User,
		Role:      profile.Role,
	}
	if err := saveCredentials(cred); err != nil {
		a.EventCh <- model.Event{Type: model.AgentReply, Message: fmt.Sprintf("login ok but failed to save credentials: %v", err)}
		return
	}

	a.bugService = bugs.NewService(bugs.NewRemoteStore(serverURL, token))
	a.issueService = issuepkg.NewService(issuepkg.NewRemoteStore(serverURL, token))
	a.projectService = projectpkg.NewService(projectpkg.NewRemoteStore(serverURL, token))
	a.issueUser = profile.User
	a.issueRole = profile.Role
	a.loginCred = cred
	runtimeKeyWarning := ""
	if strings.TrimSpace(a.Config.Model.Key) == "" {
		if strings.TrimSpace(profile.APIKey) == "" {
			runtimeKeyWarning = runtimeAPIKeyMissingMessage(serverURL)
		} else if err := a.applyRuntimeAPIKey(profile.APIKey); err != nil {
			runtimeKeyWarning = fmt.Sprintf("failed to enable server runtime api key via %s: %v", serverProfileEndpoint(serverURL), err)
		}
	}

	a.EventCh <- model.Event{Type: model.IssueUserUpdate, Message: profile.User}
	a.EventCh <- model.Event{
		Type:    model.AgentReply,
		Message: fmt.Sprintf("logged in as %s (%s)", profile.User, profile.Role),
	}
	if runtimeKeyWarning != "" {
		a.EventCh <- model.Event{
			Type:     model.ToolError,
			ToolName: "login",
			Message:  runtimeKeyWarning,
		}
	}
}

func (a *Application) ensureBugService() bool {
	if a.bugService != nil {
		return true
	}
	cred, err := loadCredentials()
	if err != nil {
		a.EventCh <- model.Event{
			Type:    model.AgentReply,
			Message: "not logged in. Run /login <token> first.",
		}
		return false
	}
	a.bugService = bugs.NewService(bugs.NewRemoteStore(cred.ServerURL, cred.Token))
	a.issueUser = cred.User
	a.issueRole = cred.Role
	a.EventCh <- model.Event{Type: model.IssueUserUpdate, Message: cred.User}
	return true
}

func (a *Application) ensureIssueService() bool {
	if a.issueService != nil {
		return true
	}
	cred, err := loadCredentials()
	if err != nil {
		a.EventCh <- model.Event{
			Type:    model.AgentReply,
			Message: "not logged in. Run /login <token> first.",
		}
		return false
	}
	a.issueService = issuepkg.NewService(issuepkg.NewRemoteStore(cred.ServerURL, cred.Token))
	a.issueUser = cred.User
	a.issueRole = cred.Role
	a.EventCh <- model.Event{Type: model.IssueUserUpdate, Message: cred.User}
	return true
}

func (a *Application) ensureProjectService() bool {
	if a.projectService != nil {
		return true
	}
	cred, err := loadCredentials()
	if err != nil {
		return false
	}
	a.projectService = projectpkg.NewService(projectpkg.NewRemoteStore(cred.ServerURL, cred.Token))
	if a.issueUser == "" {
		a.issueUser = cred.User
	}
	return true
}

func (a *Application) ensureAdmin() bool {
	if a.issueRole == "" {
		if !a.ensureBugService() {
			return false
		}
	}
	if a.issueRole != "admin" {
		a.EventCh <- model.Event{
			Type:    model.AgentReply,
			Message: "permission denied: admin role required",
		}
		return false
	}
	return true
}
