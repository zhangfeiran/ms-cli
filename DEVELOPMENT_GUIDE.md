# ms-cli 开发指南

## 快速开始

### 项目结构概览

```
ms-cli/
├── app/                    # 应用入口和引导
│   ├── main.go            # 程序入口
│   ├── bootstrap.go       # 依赖注入（需要扩展）
│   ├── wire.go            # 应用结构
│   ├── run.go             # TUI 启动
│   └── commands.go        # 命令处理
│
├── agent/                  # Agent 核心
│   ├── loop/              # 执行引擎
│   │   ├── engine.go      # ⚠️ 需要重写实现 ReAct 循环
│   │   ├── types.go       # 基础类型
│   │   ├── ports.go       # 端口定义
│   │   └── permission.go  # 权限接口
│   ├── context/           # 上下文管理
│   │   ├── manager.go     # ⚠️ 需要实现
│   │   ├── budget.go      # ⚠️ 需要扩展
│   │   └── compact.go     # ⚠️ 需要实现
│   └── memory/            # 记忆系统
│       ├── store.go       # 存储接口
│       ├── retrieve.go    # 检索
│       └── policy.go      # 策略
│
├── ui/                    # 用户界面
│   ├── app.go             # ✅ 完整
│   ├── model/             # 状态类型
│   ├── components/        # UI 组件
│   └── panels/            # 面板
│
├── integrations/          # 外部集成
│   ├── domain/            # 领域服务（保留）
│   ├── skills/            # 技能系统（保留）
│   └── llm/               # 🆕 LLM Provider（需要新建）
│       ├── provider.go
│       ├── openai/
│       └── openrouter/
│
├── tools/                 # 工具系统
│   ├── fs/                # 文件工具
│   │   └── fs.go          # ⚠️ 需要扩展
│   └── shell/             # Shell 工具
│       └── shell.go       # ⚠️ 需要扩展
│
├── executor/              # 任务执行器
│   └── runner.go          # ⚠️ 需要重写
│
├── configs/               # 配置系统
│   ├── mscli.yaml         # 主配置
│   └── ...
│
├── internal/              # 内部模块
│   └── project/           # 项目管理
│       ├── roadmap.go     # ✅ 完整
│       └── weekly.go      # ✅ 完整
│
├── trace/                 # 执行追踪
├── report/                # 报告生成
└── docs/                  # 文档
```

### 开发顺序

```
Phase 1: 基础架构 (Week 1-2)
├── 1.1 LLM Provider 架构
│   ├── integrations/llm/provider.go     # 接口定义
│   ├── integrations/llm/openai/         # OpenAI 实现
│   └── integrations/llm/openrouter/     # OpenRouter 实现
│
├── 1.2 配置系统
│   ├── configs/loader.go                # 配置加载
│   └── configs/types.go                 # 类型定义
│
└── 1.3 引导流程
    └── app/bootstrap.go                 # 扩展依赖注入

Phase 2: Agent 核心 (Week 2-3)
├── 2.1 工具系统
│   ├── tools/registry.go                # 工具注册
│   ├── tools/fs/*.go                    # 文件工具
│   └── tools/shell/*.go                 # Shell 工具
│
├── 2.2 Agent 循环
│   ├── agent/loop/engine.go             # 重写 ReAct
│   └── agent/loop/executor.go           # 执行器
│
└── 2.3 上下文管理
    └── agent/context/manager.go         # 实现管理器

Phase 3: 功能完善 (Week 3-4)
├── 3.1 权限系统
│   └── agent/loop/permission.go         # 实现权限服务
│
├── 3.2 记忆系统
│   └── agent/memory/*.go                # 实现存储
│
└── 3.3 追踪报告
    ├── trace/writer.go                  # 实现追踪
    └── report/summary.go                # 实现报告

Phase 4: 集成优化 (Week 4-5)
├── 4.1 系统集成
│   ├── app/bootstrap.go                 # 完整集成
│   └── executor/runner.go               # 实现执行器
│
└── 4.2 测试覆盖
    └── *_test.go                        # 单元测试
```

---

## 关键接口速查

### 1. LLM Provider 接口

```go
// integrations/llm/provider.go
type Provider interface {
    Name() string
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    CompleteStream(ctx context.Context, req *CompletionRequest) (StreamIterator, error)
    SupportsTools() bool
    AvailableModels() []ModelInfo
}
```

### 2. 工具接口

```go
// tools/types.go
type Tool interface {
    Name() string
    Description() string
    Schema() llm.ToolSchema
    Execute(ctx context.Context, params json.RawMessage) (*Result, error)
}
```

### 3. Agent 引擎

```go
// agent/loop/engine.go
type Engine struct { ... }

func NewEngine(cfg EngineConfig, provider llm.Provider, tools *tools.Registry) *Engine
func (e *Engine) Run(task Task) ([]Event, error)
```

---

## 配置示例

### 环境变量

```bash
# Required
export OPENAI_API_KEY="sk-..."
# 或
export OPENROUTER_API_KEY="sk-or-..."

# Optional
export MSCLI_CONFIG_PATH="~/.config/mscli/config.yaml"
export MSCLI_LOG_LEVEL="debug"
```

### 配置文件 (configs/mscli.yaml)

```yaml
model:
  provider: openai              # openai | openrouter
  model: gpt-4o-mini
  api_key: ""                   # 或使用环境变量 OPENAI_API_KEY
  temperature: 0.7
  max_tokens: 4096
  timeout_sec: 60

budget:
  max_tokens: 32768
  max_cost_usd: 10.0

ui:
  enabled: true
  show_token_bar: true

permissions:
  default_level: ask            # deny | ask | allow
  tool_policies:
    read: allow
    write: ask
    shell: ask

context:
  max_tokens: 24000
  reserve_tokens: 4000
  compaction_threshold: 0.85
  max_history_rounds: 10
```

---

## 开发规范

### 代码风格

- 遵循标准 Go 代码规范 (gofmt, golint)
- 包名使用小写，无下划线
- 接口名使用 `er` 后缀 (如 `Provider`, `Runner`)
- 错误处理使用 `fmt.Errorf("...: %w", err)`

### 测试要求

```go
// 示例测试结构
func TestEngine_Run(t *testing.T) {
    tests := []struct {
        name    string
        task    Task
        want    []Event
        wantErr bool
    }{
        // 测试用例...
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // 测试逻辑
        })
    }
}
```

### 文档要求

- 所有导出类型和函数必须有文档注释
- 复杂逻辑添加行内注释
- 接口定义附带使用示例

---

## 调试技巧

### 启用 Debug 日志

```bash
go run ./app --debug
```

### Demo 模式测试 UI

```bash
go run ./app --demo
```

### 测试单个 Provider

```bash
go test ./integrations/llm/openai -v
```

---

## 常见问题

### Q: 如何添加新的 LLM Provider?

A: 在 `integrations/llm/` 下创建新目录，实现 `Provider` 接口，在 `registry.go` 注册。

### Q: 如何添加新的工具?

A: 
1. 创建实现 `Tool` 接口的结构体
2. 在 `tools/registry.go` 中注册
3. 添加单元测试

### Q: Token 计算不准确怎么办?

A: 
- 使用官方 Tokenizer
- 预留 10% 缓冲
- 实现 `tiktoken` 兼容计算

---

## 参考文档

- [架构分析](./ARCHITECTURE_ANALYSIS.md) - 详细架构分析
- [开发计划](./DEVELOPMENT_PLAN.md) - 完整开发计划
- [接口规范](./INTERFACE_SPEC.md) - 接口详细规范
