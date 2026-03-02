# ms-cli 接口规范文档

## 1. LLM Provider 接口规范

### 1.1 核心接口定义

```go
// integrations/llm/provider.go
package llm

import (
    "context"
    "encoding/json"
    "io"
)

// Provider 是 LLM 服务的统一接口
type Provider interface {
    // Name 返回 Provider 名称
    Name() string
    
    // Complete 执行非流式补全请求
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    
    // CompleteStream 执行流式补全请求
    CompleteStream(ctx context.Context, req *CompletionRequest) (StreamIterator, error)
    
    // SupportsTools 返回是否支持工具调用
    SupportsTools() bool
    
    // AvailableModels 返回可用模型列表
    AvailableModels() []ModelInfo
}

// CompletionRequest 补全请求
type CompletionRequest struct {
    Model       string
    Messages    []Message
    Tools       []Tool        // 可选的工具定义
    Temperature float32       // 默认 0.7
    MaxTokens   int           // 默认 4096
    TopP        float32       // 默认 1.0
    Stop        []string      // 停止序列
}

// CompletionResponse 补全响应
type CompletionResponse struct {
    ID           string
    Model        string
    Content      string
    ToolCalls    []ToolCall
    FinishReason FinishReason
    Usage        Usage
}

// FinishReason 完成原因
type FinishReason string

const (
    FinishStop          FinishReason = "stop"
    FinishLength        FinishReason = "length"
    FinishToolCalls     FinishReason = "tool_calls"
    FinishContentFilter FinishReason = "content_filter"
)

// Message 对话消息
type Message struct {
    Role       string      // "system", "user", "assistant", "tool"
    Content    string
    ToolCalls  []ToolCall  // assistant 消息中的工具调用
    ToolCallID string      // tool 消息中的调用 ID
}

// Tool 工具定义
type Tool struct {
    Type     string       // 目前只有 "function"
    Function ToolFunction
}

// ToolFunction 工具函数定义
type ToolFunction struct {
    Name        string
    Description string
    Parameters  *ToolSchema  // JSON Schema
}

// ToolSchema 工具参数模式
type ToolSchema struct {
    Type       string                 `json:"type"`
    Properties map[string]Property    `json:"properties"`
    Required   []string               `json:"required"`
}

// Property 属性定义
type Property struct {
    Type        string   `json:"type"`
    Description string   `json:"description"`
    Enum        []string `json:"enum,omitempty"`
}

// ToolCall 工具调用请求
type ToolCall struct {
    ID       string       `json:"id"`
    Type     string       `json:"type"`
    Function ToolCallFunc `json:"function"`
}

// ToolCallFunc 工具调用函数信息
type ToolCallFunc struct {
    Name      string          `json:"name"`
    Arguments json.RawMessage `json:"arguments"`
}

// ToolResult 工具执行结果
type ToolResult struct {
    ToolCallID string
    Content    string
    IsError    bool
}

// Usage Token 使用情况
type Usage struct {
    PromptTokens     int
    CompletionTokens int
    TotalTokens      int
}

// ModelInfo 模型信息
type ModelInfo struct {
    ID       string
    Provider string
    MaxTokens int
}

// StreamIterator 流式响应迭代器
type StreamIterator interface {
    // Next 返回下一个 Chunk，结束时返回 io.EOF
    Next() (*StreamChunk, error)
    // Close 关闭迭代器
    Close() error
}

// StreamChunk 流式响应块
type StreamChunk struct {
    Content   string    // 增量内容
    ToolCalls []ToolCall // 增量工具调用 (可能不完整)
    FinishReason FinishReason
    Usage     *Usage    // 可能只在最后一块有
}
```

### 1.2 Provider 注册中心

```go
// integrations/llm/registry.go
package llm

// Registry 管理所有 Provider
type Registry struct {
    providers map[string]Provider
}

// NewRegistry 创建新的注册中心
func NewRegistry() *Registry

// Register 注册 Provider
func (r *Registry) Register(p Provider) error

// Get 获取指定名称的 Provider
func (r *Registry) Get(name string) (Provider, error)

// List 列出所有已注册 Provider
func (r *Registry) List() []Provider

// Default 返回默认 Provider
func (r *Registry) Default() (Provider, error)
```

### 1.3 OpenAI Provider 实现规范

```go
// integrations/llm/openai/client.go
package openai

import (
    "github.com/sashabaranov/go-openai"  // 使用官方 SDK 或自定义实现
)

type Client struct {
    apiKey   string
    baseURL  string
    model    string
    httpClient *http.Client
}

// NewClient 创建 OpenAI 客户端
func NewClient(cfg Config) (*Client, error)

func (c *Client) Name() string { return "openai" }

func (c *Client) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.CompletionResponse, error)

func (c *Client) CompleteStream(ctx context.Context, req *llm.CompletionRequest) (llm.StreamIterator, error)

func (c *Client) SupportsTools() bool { return true }

func (c *Client) AvailableModels() []llm.ModelInfo
```

### 1.4 OpenRouter Provider 实现规范

```go
// integrations/llm/openrouter/client.go
package openrouter

// OpenRouter 兼容 OpenAI API 格式，但有额外 header 要求

type Client struct {
    apiKey     string
    httpClient *http.Client
    model      string
}

// NewClient 创建 OpenRouter 客户端
func NewClient(cfg Config) (*Client, error)

// 实现与 OpenAI 类似，但添加必要的 headers:
// HTTP-Referer: <site_url>
// X-Title: <site_name>
```

---

## 2. 工具系统接口规范

### 2.1 核心接口

```go
// tools/types.go
package tools

import (
    "context"
    "encoding/json"
)

// Tool 是可执行工具的接口
type Tool interface {
    // Name 返回工具名称 (英文，无空格)
    Name() string
    
    // Description 返回工具描述 (用于 LLM 理解)
    Description() string
    
    // Schema 返回工具参数的模式定义
    Schema() llm.ToolSchema
    
    // Execute 执行工具
    Execute(ctx context.Context, params json.RawMessage) (*Result, error)
}

// Result 是工具执行结果
type Result struct {
    Content string  // 主要输出内容
    Summary string  // 用于 UI 显示的摘要 (如 "42 lines", "5 matches")
    Error   error   // 执行错误
}

// StringResult 快速创建字符串结果
func StringResult(content string) *Result

// ErrorResult 快速创建错误结果
func ErrorResult(err error) *Result
```

### 2.2 工具注册中心

```go
// tools/registry.go
package tools

// Registry 管理所有可用工具
type Registry struct {
    tools map[string]Tool
}

// NewRegistry 创建工具注册中心
func NewRegistry() *Registry

// Register 注册工具
func (r *Registry) Register(t Tool) error

// Get 获取指定名称的工具
func (r *Registry) Get(name string) (Tool, bool)

// List 列出所有工具
func (r *Registry) List() []Tool

// ToLLMTools 转换为 LLM 工具定义格式
func (r *Registry) ToLLMTools() []llm.Tool
```

### 2.3 文件工具规范

```go
// tools/fs/read.go
package fs

// ReadTool 读取文件内容
type ReadTool struct {
    workDir string
}

func NewReadTool(workDir string) *ReadTool

func (t *ReadTool) Name() string { return "read" }

func (t *ReadTool) Description() string {
    return "Read the contents of a file. Use this when you need to examine file contents."
}

func (t *ReadTool) Schema() llm.ToolSchema {
    return llm.ToolSchema{
        Type: "object",
        Properties: map[string]llm.Property{
            "path": {
                Type:        "string",
                Description: "Relative path to the file to read",
            },
            "offset": {
                Type:        "integer",
                Description: "Line number to start reading from (1-indexed)",
            },
            "limit": {
                Type:        "integer",
                Description: "Maximum number of lines to read",
            },
        },
        Required: []string{"path"},
    }
}

type readParams struct {
    Path   string `json:"path"`
    Offset int    `json:"offset"`
    Limit  int    `json:"limit"`
}

func (t *ReadTool) Execute(ctx context.Context, params json.RawMessage) (*tools.Result, error)
```

```go
// tools/fs/write.go
package fs

// WriteTool 写入文件内容
type WriteTool struct {
    workDir string
}

func NewWriteTool(workDir string) *WriteTool

func (t *WriteTool) Name() string { return "write" }

func (t *WriteTool) Description() string {
    return "Create a new file or overwrite an existing file with new content."
}

func (t *WriteTool) Schema() llm.ToolSchema {
    return llm.ToolSchema{
        Type: "object",
        Properties: map[string]llm.Property{
            "path": {
                Type:        "string",
                Description: "Relative path to the file to write",
            },
            "content": {
                Type:        "string",
                Description: "Content to write to the file",
            },
        },
        Required: []string{"path", "content"},
    }
}
```

```go
// tools/fs/edit.go
package fs

// EditTool 编辑文件内容 (查找替换)
type EditTool struct {
    workDir string
}

func NewEditTool(workDir string) *EditTool

func (t *EditTool) Name() string { return "edit" }

func (t *EditTool) Description() string {
    return "Edit a file by replacing specific text. Use this for making targeted changes."
}

func (t *EditTool) Schema() llm.ToolSchema {
    return llm.ToolSchema{
        Type: "object",
        Properties: map[string]llm.Property{
            "path": {
                Type:        "string",
                Description: "Relative path to the file to edit",
            },
            "old_string": {
                Type:        "string",
                Description: "Exact text to replace (must match exactly including whitespace)",
            },
            "new_string": {
                Type:        "string",
                Description: "New text to replace the old_string with",
            },
        },
        Required: []string{"path", "old_string", "new_string"},
    }
}
```

```go
// tools/fs/grep.go
package fs

// GrepTool 搜索文件内容
type GrepTool struct {
    workDir string
}

func NewGrepTool(workDir string) *GrepTool

func (t *GrepTool) Name() string { return "grep" }

func (t *GrepTool) Description() string {
    return "Search for patterns in files using regular expressions."
}

func (t *GrepTool) Schema() llm.ToolSchema {
    return llm.ToolSchema{
        Type: "object",
        Properties: map[string]llm.Property{
            "pattern": {
                Type:        "string",
                Description: "Regular expression pattern to search for",
            },
            "path": {
                Type:        "string",
                Description: "Directory or file to search in (default: current directory)",
            },
            "include": {
                Type:        "string",
                Description: "File pattern to include (e.g., '*.go')",
            },
        },
        Required: []string{"pattern"},
    }
}
```

```go
// tools/fs/glob.go
package fs

// GlobTool 文件模式匹配
type GlobTool struct {
    workDir string
}

func NewGlobTool(workDir string) *GlobTool

func (t *GlobTool) Name() string { return "glob" }

func (t *GlobTool) Description() string {
    return "Find files matching a glob pattern. Use this to explore project structure."
}

func (t *GlobTool) Schema() llm.ToolSchema {
    return llm.ToolSchema{
        Type: "object",
        Properties: map[string]llm.Property{
            "pattern": {
                Type:        "string",
                Description: "Glob pattern (e.g., '*.go', '**/*.yaml')",
            },
        },
        Required: []string{"pattern"},
    }
}
```

### 2.4 Shell 工具规范

```go
// tools/shell/runner.go
package shell

import (
    "context"
    "os/exec"
    "time"
)

// Runner 执行 Shell 命令
type Runner struct {
    workDir       string
    timeout       time.Duration
    allowedCmds   []string  // 白名单命令
    blockedCmds   []string  // 黑名单命令
    requireConfirm []string  // 需要确认的危险命令
}

// NewRunner 创建 Shell 执行器
func NewRunner(cfg Config) *Runner

// Run 执行命令并返回输出
func (r *Runner) Run(ctx context.Context, cmd string) (*Result, error)

// IsAllowed 检查命令是否允许执行
func (r *Runner) IsAllowed(cmd string) (bool, string) // (allowed, reason)
```

```go
// tools/shell/shell.go
package shell

import (
    "encoding/json"
    "github.com/vigo999/ms-cli/integrations/llm"
    "github.com/vigo999/ms-cli/tools"
)

// ShellTool 包装 Runner 为 Tool 接口
type ShellTool struct {
    runner *Runner
}

func NewShellTool(runner *Runner) *ShellTool

func (t *ShellTool) Name() string { return "shell" }

func (t *ShellTool) Description() string {
    return "Execute a shell command. Use this for running tests, building, git operations, etc."
}

func (t *ShellTool) Schema() llm.ToolSchema {
    return llm.ToolSchema{
        Type: "object",
        Properties: map[string]llm.Property{
            "command": {
                Type:        "string",
                Description: "The shell command to execute",
            },
            "timeout": {
                Type:        "integer",
                Description: "Timeout in seconds (default: 60)",
            },
        },
        Required: []string{"command"},
    }
}

type shellParams struct {
    Command string `json:"command"`
    Timeout int    `json:"timeout"`
}

func (t *ShellTool) Execute(ctx context.Context, params json.RawMessage) (*tools.Result, error)
```

---

## 3. Agent Loop 接口规范

### 3.1 引擎接口

```go
// agent/loop/engine.go
package loop

import (
    "context"
)

// Engine 驱动任务执行的引擎
type Engine struct {
    provider    llm.Provider
    tools       *tools.Registry
    contextMgr  *context.Manager
    permission  PermissionService
    config      EngineConfig
}

// EngineConfig 引擎配置
type EngineConfig struct {
    MaxIterations    int           // 最大循环次数 (默认: 10)
    MaxTokens        int           // Token 上限
    Temperature      float32       // 温度参数
    TimeoutPerTurn   time.Duration // 每轮超时
    SystemPrompt     string        // 系统提示词
}

// NewEngine 创建引擎实例
func NewEngine(cfg EngineConfig, provider llm.Provider, tools *tools.Registry) *Engine

// Run 执行任务并返回事件序列
func (e *Engine) Run(task Task) ([]Event, error)

// RunWithContext 支持取消和超时的执行
func (e *Engine) RunWithContext(ctx context.Context, task Task) ([]Event, error)
```

### 3.2 执行上下文

```go
// agent/loop/executor.go
package loop

// executor 管理单个任务的执行循环
type executor struct {
    engine      *Engine
    task        Task
    messages    []llm.Message
    events      []Event
    iterations  int
    usage       llm.Usage
}

// run 执行完整的 ReAct 循环
func (ex *executor) run(ctx context.Context) ([]Event, error)

// step 执行单轮迭代
func (ex *executor) step(ctx context.Context) (continueLoop bool, err error)

// buildSystemPrompt 构建系统提示词
func (ex *executor) buildSystemPrompt() llm.Message

// handleLLMResponse 处理 LLM 响应
func (ex *executor) handleLLMResponse(resp *llm.CompletionResponse) error

// executeToolCalls 执行工具调用
func (ex *executor) executeToolCalls(ctx context.Context, calls []llm.ToolCall) ([]llm.ToolResult, error)

// addEvent 添加事件到序列
func (ex *executor) addEvent(ev Event)
```

### 3.3 扩展类型定义

```go
// agent/loop/types.go
package loop

// Task 用户任务
type Task struct {
    ID          string
    Description string
    Context     map[string]string  // 额外上下文
}

// Event 引擎事件 (扩展 UI model.Event)
type Event struct {
    Type       EventType
    Task       string
    Message    string
    ToolName   string
    Summary    string
    CtxUsed    int
    TokensUsed int
    Usage      llm.Usage  // Token 使用详情
}

// EventType 事件类型 (与 UI model 保持一致)
type EventType string

const (
    // 执行事件
    TaskStarted   EventType = "TaskStarted"
    TaskCompleted EventType = "TaskCompleted"
    TaskFailed    EventType = "TaskFailed"
    
    // LLM 事件
    LLMThinking   EventType = "LLMThinking"
    LLMResponse   EventType = "LLMResponse"
    
    // 工具事件
    ToolStarted   EventType = "ToolStarted"
    ToolCompleted EventType = "ToolCompleted"
    ToolError     EventType = "ToolError"
    
    // 保留 UI 兼容的事件类型
    CmdStarted    EventType = "CmdStarted"
    CmdOutput     EventType = "CmdOutput"
    CmdFinished   EventType = "CmdFinished"
    AgentReply    EventType = "AgentReply"
    AgentThinking EventType = "AgentThinking"
    TokenUpdate   EventType = "TokenUpdate"
    ToolRead      EventType = "ToolRead"
    ToolGrep      EventType = "ToolGrep"
    ToolGlob      EventType = "ToolGlob"
    ToolEdit      EventType = "ToolEdit"
    ToolWrite     EventType = "ToolWrite"
    AnalysisReady EventType = "AnalysisReady"
    Done          EventType = "Done"
)
```

---

## 4. 上下文管理接口规范

```go
// agent/context/manager.go
package context

import (
    "github.com/vigo999/ms-cli/integrations/llm"
)

// Manager 管理对话上下文
type Manager struct {
    budget   *Budget
    messages []llm.Message
    config   ManagerConfig
}

// ManagerConfig 管理器配置
type ManagerConfig struct {
    MaxTokens           int     // 最大 Token 限制
    ReserveTokens       int     // 预留 Token (用于响应)
    CompactionThreshold float64 // 压缩触发阈值 (0-1)
    MaxHistoryRounds    int     // 保留的最大对话轮数
}

// NewManager 创建上下文管理器
func NewManager(cfg ManagerConfig) *Manager

// AddMessage 添加消息，自动处理压缩
func (m *Manager) AddMessage(msg llm.Message) error

// GetMessages 获取当前所有消息 (已考虑预算)
func (m *Manager) GetMessages() []llm.Message

// GetSystemPrompt 获取系统提示词
func (m *Manager) GetSystemPrompt() *llm.Message

// SetSystemPrompt 设置系统提示词
func (m *Manager) SetSystemPrompt(content string)

// AddToolResult 添加工具执行结果
func (m *Manager) AddToolResult(callID string, content string)

// EstimateTokens 估算消息列表的 Token 数
func (m *Manager) EstimateTokens(msgs []llm.Message) int

// Compact 手动触发压缩
func (m *Manager) Compact() error

// Clear 清空上下文 (保留 System Prompt)
func (m *Manager) Clear()

// TokenUsage 返回当前 Token 使用情况
func (m *Manager) TokenUsage() TokenUsage

// TokenUsage Token 使用统计
type TokenUsage struct {
    Current    int
    Max        int
    Reserved   int
    Available  int
}
```

---

## 5. 权限服务接口规范

```go
// agent/loop/permission.go
package loop

import (
    "context"
)

// PermissionService 权限控制服务
type PermissionService interface {
    // Request 请求执行权限
    // tool: 工具名称
    // action: 具体操作 (如命令内容)
    // path: 相关路径
    // 返回值: (是否允许, 错误)
    Request(ctx context.Context, tool, action, path string) (bool, error)
    
    // Check 检查是否已有权限 (不触发交互)
    Check(tool, action string) PermissionLevel
    
    // Grant 授予权限
    Grant(tool string, level PermissionLevel)
    
    // Revoke 撤销权限
    Revoke(tool string)
}

// PermissionLevel 权限级别
type PermissionLevel int

const (
    PermissionDeny PermissionLevel = iota
    PermissionAsk
    PermissionAllowOnce
    PermissionAllowSession
    PermissionAllowAlways
)

// DefaultPermissionService 默认实现
type DefaultPermissionService struct {
    policies map[string]PermissionLevel
    ui       PermissionUI
}

// PermissionUI 权限请求 UI 接口
type PermissionUI interface {
    // RequestPermission 向用户请求权限
    RequestPermission(tool, action, path string) (granted bool, remember bool, err error)
}
```

---

## 6. 配置系统接口规范

```go
// configs/types.go
package configs

// Config 完整配置
type Config struct {
    Model       ModelConfig       `yaml:"model"`
    Budget      BudgetConfig      `yaml:"budget"`
    UI          UIConfig          `yaml:"ui"`
    Permissions PermissionsConfig `yaml:"permissions"`
    Context     ContextConfig     `yaml:"context"`
    Memory      MemoryConfig      `yaml:"memory"`
    Skills      SkillsConfig      `yaml:"skills"`
    Execution   ExecutionConfig   `yaml:"execution"`
}

// ModelConfig 模型配置
type ModelConfig struct {
    Provider    string            `yaml:"provider"`              // openai, openrouter
    Endpoint    string            `yaml:"endpoint,omitempty"`
    APIKey      string            `yaml:"api_key,omitempty"`
    Model       string            `yaml:"model"`
    Temperature float64           `yaml:"temperature"`
    MaxTokens   int               `yaml:"max_tokens"`
    TimeoutSec  int               `yaml:"timeout_sec"`
    Headers     map[string]string `yaml:"headers,omitempty"`
}

// BudgetConfig 预算配置
type BudgetConfig struct {
    MaxTokens   int     `yaml:"max_tokens"`
    MaxCostUSD  float64 `yaml:"max_cost_usd"`
    DailyLimit  int     `yaml:"daily_limit,omitempty"`
}

// UIConfig UI 配置
type UIConfig struct {
    Enabled       bool   `yaml:"enabled"`
    Theme         string `yaml:"theme,omitempty"`
    ShowTokenBar  bool   `yaml:"show_token_bar"`
    Animation     bool   `yaml:"animation"`
}

// PermissionsConfig 权限配置
type PermissionsConfig struct {
    SkipRequests bool              `yaml:"skip_requests"`
    DefaultLevel string            `yaml:"default_level"` // deny, ask, allow
    ToolPolicies map[string]string `yaml:"tool_policies,omitempty"`
    AllowedTools []string          `yaml:"allowed_tools"`
}

// ContextConfig 上下文配置
type ContextConfig struct {
    MaxTokens           int     `yaml:"max_tokens"`
    ReserveTokens       int     `yaml:"reserve_tokens"`
    CompactionThreshold float64 `yaml:"compaction_threshold"`
    MaxHistoryRounds    int     `yaml:"max_history_rounds"`
}

// MemoryConfig 记忆配置
type MemoryConfig struct {
    Enabled   bool   `yaml:"enabled"`
    StorePath string `yaml:"store_path,omitempty"`
    MaxItems  int    `yaml:"max_items"`
    MaxBytes  int64  `yaml:"max_bytes"`
    TTLHours  int    `yaml:"ttl_hours"`
}

// SkillsConfig 技能配置
type SkillsConfig struct {
    Repo      string   `yaml:"repo"`
    Revision  string   `yaml:"revision"`
    CacheDir  string   `yaml:"cache_dir"`
    Workflows []string `yaml:"workflows"`
}

// ExecutionConfig 执行配置
type ExecutionConfig struct {
    Mode           string            `yaml:"mode"`            // local, docker
    TimeoutSec     int               `yaml:"timeout_sec"`
    MaxConcurrency int               `yaml:"max_concurrency"`
    Docker         DockerConfig      `yaml:"docker,omitempty"`
}

// DockerConfig Docker 配置
type DockerConfig struct {
    Image   string            `yaml:"image"`
    CPU     string            `yaml:"cpu"`
    Memory  string            `yaml:"memory"`
    Network string            `yaml:"network"`
    Env     map[string]string `yaml:"env,omitempty"`
}
```

```go
// configs/loader.go
package configs

import (
    "os"
    "path/filepath"
    "gopkg.in/yaml.v3"
)

// LoadFromFile 从文件加载配置
func LoadFromFile(path string) (*Config, error)

// LoadWithEnv 加载配置并应用环境变量覆盖
func LoadWithEnv(path string) (*Config, error)

// DefaultConfig 返回默认配置
func DefaultConfig() *Config

// applyEnvOverrides 应用环境变量覆盖
func applyEnvOverrides(cfg *Config)

// Validate 验证配置有效性
func (c *Config) Validate() error
```

---

## 7. Bootstrap 集成规范

```go
// app/bootstrap.go
package main

import (
    "github.com/vigo999/ms-cli/agent/context"
    "github.com/vigo999/ms-cli/agent/loop"
    "github.com/vigo999/ms-cli/configs"
    "github.com/vigo999/ms-cli/integrations/llm"
    "github.com/vigo999/ms-cli/tools"
    "github.com/vigo999/ms-cli/tools/fs"
    "github.com/vigo999/ms-cli/tools/shell"
)

// Bootstrap 完整引导流程
func Bootstrap(demo bool, configPath string) (*Application, error) {
    // 1. 加载配置
    cfg, err := configs.LoadWithEnv(configPath)
    if err != nil {
        return nil, fmt.Errorf("load config: %w", err)
    }
    
    // 2. 初始化 Provider
    provider, err := initProvider(cfg.Model)
    if err != nil {
        return nil, fmt.Errorf("init provider: %w", err)
    }
    
    // 3. 初始化工具注册中心
    toolRegistry := initTools(cfg, workDir)
    
    // 4. 初始化上下文管理器
    ctxManager := context.NewManager(context.ManagerConfig{
        MaxTokens:           cfg.Context.MaxTokens,
        ReserveTokens:       cfg.Context.ReserveTokens,
        CompactionThreshold: cfg.Context.CompactionThreshold,
        MaxHistoryRounds:    cfg.Context.MaxHistoryRounds,
    })
    
    // 5. 初始化权限服务
    permService := initPermissionService(cfg.Permissions)
    
    // 6. 初始化引擎
    engineCfg := loop.EngineConfig{
        MaxIterations:  cfg.Execution.MaxConcurrency * 5,
        MaxTokens:      cfg.Budget.MaxTokens,
        Temperature:    float32(cfg.Model.Temperature),
        TimeoutPerTurn: time.Duration(cfg.Model.TimeoutSec) * time.Second,
    }
    engine := loop.NewEngine(engineCfg, provider, toolRegistry)
    engine.SetContextManager(ctxManager)
    engine.SetPermissionService(permService)
    
    // 7. 创建应用
    return &Application{
        Engine:  engine,
        EventCh: make(chan model.Event, 64),
        Demo:    demo,
        WorkDir: workDir,
        RepoURL: "github.com/vigo999/ms-cli",
        Config:  cfg,
    }, nil
}

// initProvider 初始化 LLM Provider
func initProvider(cfg configs.ModelConfig) (llm.Provider, error) {
    switch cfg.Provider {
    case "openai":
        return llmopenai.NewClient(llmopenai.Config{
            APIKey:   getAPIKey(cfg.APIKey, "OPENAI_API_KEY"),
            Endpoint: cfg.Endpoint,
            Model:    cfg.Model,
        })
    case "openrouter":
        return llmopenrouter.NewClient(llmopenrouter.Config{
            APIKey:   getAPIKey(cfg.APIKey, "OPENROUTER_API_KEY"),
            Model:    cfg.Model,
        })
    default:
        return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
    }
}

// initTools 初始化工具注册中心
func initTools(cfg *configs.Config, workDir string) *tools.Registry {
    registry := tools.NewRegistry()
    
    // 注册文件工具
    registry.Register(fs.NewReadTool(workDir))
    registry.Register(fs.NewWriteTool(workDir))
    registry.Register(fs.NewEditTool(workDir))
    registry.Register(fs.NewGrepTool(workDir))
    registry.Register(fs.NewGlobTool(workDir))
    
    // 注册 Shell 工具
    shellRunner := shell.NewRunner(shell.Config{
        WorkDir:       workDir,
        Timeout:       time.Duration(cfg.Execution.TimeoutSec) * time.Second,
        AllowedCmds:   cfg.Permissions.AllowedTools,
    })
    registry.Register(shell.NewShellTool(shellRunner))
    
    return registry
}

// getAPIKey 获取 API Key (配置 > 环境变量)
func getAPIKey(configKey, envVar string) string {
    if configKey != "" {
        return configKey
    }
    return os.Getenv(envVar)
}
```
