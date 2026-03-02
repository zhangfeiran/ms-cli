package loop

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/vigo999/ms-cli/configs"
)

// PermissionLevel represents the permission level for a tool.
type PermissionLevel int

const (
	// PermissionDeny always denies the tool.
	PermissionDeny PermissionLevel = iota
	// PermissionAsk asks user for permission each time.
	PermissionAsk
	// PermissionAllowOnce allows once without asking.
	PermissionAllowOnce
	// PermissionAllowSession allows for the current session.
	PermissionAllowSession
	// PermissionAllowAlways always allows.
	PermissionAllowAlways
)

// String returns the string representation.
func (p PermissionLevel) String() string {
	switch p {
	case PermissionDeny:
		return "deny"
	case PermissionAsk:
		return "ask"
	case PermissionAllowOnce:
		return "allow_once"
	case PermissionAllowSession:
		return "allow_session"
	case PermissionAllowAlways:
		return "allow_always"
	default:
		return "unknown"
	}
}

// ParsePermissionLevel parses a permission level string.
func ParsePermissionLevel(s string) PermissionLevel {
	switch strings.ToLower(s) {
	case "deny":
		return PermissionDeny
	case "ask":
		return PermissionAsk
	case "allow_once":
		return PermissionAllowOnce
	case "allow_session":
		return PermissionAllowSession
	case "allow_always", "allow":
		return PermissionAllowAlways
	default:
		return PermissionAsk
	}
}

// PermissionService controls tool-call permissions.
type PermissionService interface {
	// Request requests permission to execute a tool.
	// Returns (granted, error).
	Request(ctx context.Context, tool, action, path string) (bool, error)

	// Check checks the current permission level without triggering interaction.
	Check(tool, action string) PermissionLevel

	// Grant grants permission for a tool.
	Grant(tool string, level PermissionLevel)

	// Revoke revokes permission for a tool.
	Revoke(tool string)
}

// DefaultPermissionService is the default permission service implementation.
type DefaultPermissionService struct {
	mu       sync.RWMutex
	policies map[string]PermissionLevel
	default_ PermissionLevel
	skipAsk  bool
	ui       PermissionUI
}

// PermissionUI is the interface for permission UI interaction.
type PermissionUI interface {
	// RequestPermission asks the user for permission.
	// Returns (granted, remember, error).
	RequestPermission(tool, action, path string) (bool, bool, error)
}

// NewDefaultPermissionService creates a new permission service.
func NewDefaultPermissionService(cfg configs.PermissionsConfig) *DefaultPermissionService {
	svc := &DefaultPermissionService{
		policies: make(map[string]PermissionLevel),
		default_: ParsePermissionLevel(cfg.DefaultLevel),
		skipAsk:  cfg.SkipRequests,
	}

	// Load tool policies
	for tool, level := range cfg.ToolPolicies {
		svc.policies[tool] = ParsePermissionLevel(level)
	}

	// Load allowed tools as allow_always
	for _, tool := range cfg.AllowedTools {
		svc.policies[tool] = PermissionAllowAlways
	}

	return svc
}

// SetUI sets the permission UI.
func (s *DefaultPermissionService) SetUI(ui PermissionUI) {
	s.ui = ui
}

// Request requests permission.
func (s *DefaultPermissionService) Request(ctx context.Context, tool, action, path string) (bool, error) {
	level := s.Check(tool, action)

	switch level {
	case PermissionDeny:
		return false, fmt.Errorf("tool %q is blocked", tool)

	case PermissionAllowAlways, PermissionAllowSession:
		return true, nil

	case PermissionAllowOnce:
		// Grant once then revert to ask
		s.Grant(tool, PermissionAsk)
		return true, nil

	case PermissionAsk:
		if s.skipAsk {
			return true, nil
		}

		// Interactive permission request
		if s.ui != nil {
			granted, remember, err := s.ui.RequestPermission(tool, action, path)
			if err != nil {
				return false, err
			}

			if granted && remember {
				s.Grant(tool, PermissionAllowSession)
			}

			return granted, nil
		}

		// No UI, default to allow
		return true, nil
	}

	return false, fmt.Errorf("unknown permission level")
}

// Check checks the permission level.
func (s *DefaultPermissionService) Check(tool, action string) PermissionLevel {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check specific tool policy
	if level, ok := s.policies[tool]; ok {
		return level
	}

	// Check read/write patterns
	if tool == "read" || tool == "glob" {
		return PermissionAllowAlways
	}

	// Check destructive commands
	if tool == "write" || tool == "edit" {
		return maxPermission(s.default_, PermissionAsk)
	}

	if tool == "shell" {
		return maxPermission(s.default_, PermissionAsk)
	}

	return s.default_
}

// Grant grants permission.
func (s *DefaultPermissionService) Grant(tool string, level PermissionLevel) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.policies[tool] = level
}

// Revoke revokes permission.
func (s *DefaultPermissionService) Revoke(tool string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.policies, tool)
}

// GetPolicies returns a copy of all policies.
func (s *DefaultPermissionService) GetPolicies() map[string]PermissionLevel {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]PermissionLevel, len(s.policies))
	for k, v := range s.policies {
		result[k] = v
	}
	return result
}

func maxPermission(a, b PermissionLevel) PermissionLevel {
	if a > b {
		return a
	}
	return b
}

// NoOpPermissionService is a permission service that always allows.
type NoOpPermissionService struct{}

// NewNoOpPermissionService creates a no-op permission service.
func NewNoOpPermissionService() *NoOpPermissionService {
	return &NoOpPermissionService{}
}

// Request always grants permission.
func (s *NoOpPermissionService) Request(ctx context.Context, tool, action, path string) (bool, error) {
	return true, nil
}

// Check always returns allow always.
func (s *NoOpPermissionService) Check(tool, action string) PermissionLevel {
	return PermissionAllowAlways
}

// Grant is a no-op.
func (s *NoOpPermissionService) Grant(tool string, level PermissionLevel) {}

// Revoke is a no-op.
func (s *NoOpPermissionService) Revoke(tool string) {}
