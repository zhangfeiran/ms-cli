package context

import (
	"fmt"
	"sync"

	"github.com/vigo999/ms-cli/integrations/llm"
)

// ManagerConfig holds the manager configuration.
type ManagerConfig struct {
	MaxTokens           int
	ReserveTokens       int
	CompactionThreshold float64
	MaxHistoryRounds    int
}

// Manager manages conversation context.
type Manager struct {
	config   ManagerConfig
	mu       sync.RWMutex
	messages []llm.Message
	system   *llm.Message
	usage    TokenUsage
}

// TokenUsage represents token usage statistics.
type TokenUsage struct {
	Current   int
	Max       int
	Reserved  int
	Available int
}

// NewManager creates a new context manager.
func NewManager(cfg ManagerConfig) *Manager {
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 24000
	}
	if cfg.ReserveTokens == 0 {
		cfg.ReserveTokens = 4000
	}
	if cfg.CompactionThreshold == 0 {
		cfg.CompactionThreshold = 0.85
	}
	if cfg.MaxHistoryRounds == 0 {
		cfg.MaxHistoryRounds = 10
	}

	return &Manager{
		config:   cfg,
		messages: make([]llm.Message, 0),
		usage: TokenUsage{
			Max:      cfg.MaxTokens,
			Reserved: cfg.ReserveTokens,
			Available: cfg.MaxTokens - cfg.ReserveTokens,
		},
	}
}

// SetSystemPrompt sets the system prompt.
func (m *Manager) SetSystemPrompt(content string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	msg := llm.NewSystemMessage(content)
	m.system = &msg
	m.recalculateUsage()
}

// GetSystemPrompt returns the system prompt.
func (m *Manager) GetSystemPrompt() *llm.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.system == nil {
		return nil
	}
	msg := *m.system
	return &msg
}

// AddMessage adds a message to the context.
func (m *Manager) AddMessage(msg llm.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if we need to compact
	estimatedTokens := m.estimateTokens(m.messages) + m.estimateSingle(msg)
	if float64(estimatedTokens) > float64(m.config.MaxTokens)*m.config.CompactionThreshold {
		if err := m.compactLocked(); err != nil {
			return fmt.Errorf("compact context: %w", err)
		}
	}

	m.messages = append(m.messages, msg)
	m.recalculateUsage()
	return nil
}

// AddToolResult adds a tool result to the context.
func (m *Manager) AddToolResult(callID, content string) error {
	return m.AddMessage(llm.NewToolMessage(callID, content))
}

// GetMessages returns all messages including system prompt.
func (m *Manager) GetMessages() []llm.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]llm.Message, 0, len(m.messages)+1)
	if m.system != nil {
		result = append(result, *m.system)
	}
	result = append(result, m.messages...)
	return result
}

// GetNonSystemMessages returns only non-system messages.
func (m *Manager) GetNonSystemMessages() []llm.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]llm.Message, len(m.messages))
	copy(result, m.messages)
	return result
}

// Clear clears all messages except system prompt.
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.messages = make([]llm.Message, 0)
	m.recalculateUsage()
}

// Compact manually triggers context compaction.
func (m *Manager) Compact() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.compactLocked()
}

// TokenUsage returns current token usage.
func (m *Manager) TokenUsage() TokenUsage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.usage
}

// EstimateTokens estimates token count for messages.
func (m *Manager) EstimateTokens(msgs []llm.Message) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.estimateTokens(msgs)
}

// IsWithinBudget checks if adding a message would exceed budget.
func (m *Manager) IsWithinBudget(msg llm.Message) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	estimated := m.estimateTokens(m.messages) + m.estimateSingle(msg)
	return estimated <= m.config.MaxTokens-m.config.ReserveTokens
}

// compactLocked compacts the context (must hold lock).
func (m *Manager) compactLocked() error {
	// Strategy: Keep system prompt, recent messages, and summarize older ones
	if len(m.messages) <= m.config.MaxHistoryRounds {
		return nil
	}

	// Keep last N rounds (each round = user + assistant + optional tool calls)
	keepCount := m.config.MaxHistoryRounds * 2
	if keepCount > len(m.messages) {
		keepCount = len(m.messages)
	}

	// Messages to remove
	removed := m.messages[:len(m.messages)-keepCount]
	m.messages = m.messages[len(m.messages)-keepCount:]

	// Create summary of removed messages
	if len(removed) > 0 {
		summary := fmt.Sprintf("[Earlier conversation: %d messages summarized]", len(removed))
		summaryMsg := llm.NewSystemMessage(summary)
		// Insert after system prompt (which is handled separately)
		m.messages = append([]llm.Message{summaryMsg}, m.messages...)
	}

	m.recalculateUsage()
	return nil
}

// estimateTokens estimates token count (simple heuristic).
func (m *Manager) estimateTokens(msgs []llm.Message) int {
	total := 0
	for _, msg := range msgs {
		total += m.estimateSingle(msg)
	}
	return total
}

// estimateSingle estimates tokens for a single message.
func (m *Manager) estimateSingle(msg llm.Message) int {
	// Simple estimation: ~4 characters per token on average
	// More accurate estimation would use tiktoken or similar
	content := msg.Content
	for _, tc := range msg.ToolCalls {
		content += tc.Function.Name
		content += string(tc.Function.Arguments)
	}
	return len(content)/4 + 10 // Base overhead per message
}

// recalculateUsage recalculates token usage (must hold lock).
func (m *Manager) recalculateUsage() {
	total := m.estimateTokens(m.messages)
	if m.system != nil {
		total += m.estimateSingle(*m.system)
	}

	m.usage = TokenUsage{
		Current:   total,
		Max:       m.config.MaxTokens,
		Reserved:  m.config.ReserveTokens,
		Available: m.config.MaxTokens - total - m.config.ReserveTokens,
	}
}

// GetStats returns context statistics.
func (m *Manager) GetStats() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]any{
		"total_messages":    len(m.messages),
		"has_system_prompt": m.system != nil,
		"token_usage":       m.usage,
		"max_tokens":        m.config.MaxTokens,
	}
}
