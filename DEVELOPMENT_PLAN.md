# ms-cli 开发计划

## 目标
构建一个功能完整的 AI Agent CLI 工具，支持多 LLM Provider (OpenAI, OpenRouter 等)，保持现有目录结构，实现端到端的任务执行能力。

---

## Phase 1: 基础架构建设 (Week 1-2)

### 1.1 LLM Provider 架构设计

**目标**: 设计并实现可扩展的 LLM Provider 系统

**设计原则**:
- 统一接口，支持多 Provider 无缝切换
- Provider 级配置隔离
- 支持流式响应
- 工具调用 (Function Calling) 标准化

**新建文件**:
```
integrations/
├── llm/
│   ├── provider.go        # Provider 接口定义
│   ├── config.go          # Provider 配置
│   ├── openai/
│   │   ├── client.go      # OpenAI 实现
│   │   └── config.go      # OpenAI 配置
│   ├── openrouter/
│   │   ├── client.go      # OpenRouter 实现
│   │   └── config.go      # OpenRouter 配置
│   └── registry.go        # Provider 注册中心
```

**接口设计**:
```go
// integrations/llm/provider.go
package llm

type Provider interface {
    Name() string
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    CompleteStream(ctx context.Context, req *CompletionRequest) (StreamIterator, error)
    SupportsTools() bool
}

type CompletionRequest struct {
    Model       string
    Messages    []Message
    Tools       []Tool
    Temperature float32
    MaxTokens   int
}

type Message struct {
    Role    string      // system, user, assistant, tool
    Content string
    ToolCalls []ToolCall
}

type Tool struct {
    Type     string
    Function ToolFunction
}

type ToolFunction struct {
    Name        string
    Description string
    Parameters  json.RawMessage
}
```

**验收标准**:
- [ ] 定义 Provider 接口
- [ ] 实现 OpenAI Provider (chat.completions)
- [ ] 实现 OpenRouter Provider
- [ ] 支持流式响应
- [ ] 实现 Provider 配置加载
- [ ] 单元测试覆盖率 > 60%

---

### 1.2 配置管理系统

**目标**: 实现统一的配置加载与管理

**新建文件**:
```
configs/
├── loader.go          # 配置加载器
├── types.go           # 配置类型定义
└── validator.go       # 配置验证
```

**配置结构**:
```go
// configs/types.go
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

type ModelConfig struct {
    Provider    string            `yaml:"provider"`     // openai, openrouter
    Endpoint    string            `yaml:"endpoint"`     // API 端点
    APIKey      string            `yaml:"api_key"`      // 或从环境变量读取
    Model       string            `yaml:"model"`        // 模型名称
    Temperature float64           `yaml:"temperature"`  // 温度参数
    MaxTokens   int               `yaml:"max_tokens"`   // 最大 Token
    TimeoutSec  int               `yaml:"timeout_sec"`  // 超时时间
    Headers     map[string]string `yaml:"headers"`      // 额外请求头
}
```

**验收标准**:
- [ ] 支持 YAML 配置文件加载
- [ ] 支持环境变量覆盖 (MSCLI_API_KEY 等)
- [ ] 配置验证与错误提示
- [ ] 热重载支持 (可选)

---

## Phase 2: Agent 核心实现 (Week 2-3)

### 2.1 工具系统 (Tools)

**目标**: 实现 Agent 可调用的工具集

**新建/修改文件**:
```
tools/
├── registry.go           # 工具注册中心
├── executor.go           # 工具执行器
├── types.go              # 工具类型定义
├── fs/
│   ├── fs.go            # 现有文件 -> 扩展实现
│   ├── read.go          # 文件读取
│   ├── write.go         # 文件写入
│   ├── edit.go          # 文件编辑
│   └── grep.go          # 内容搜索
├── shell/
│   ├── shell.go         # 现有文件 -> 扩展实现
│   └── runner.go        # 命令执行
└── go/
    └── runner.go        # Go 特定工具 (测试、构建)
```

**工具定义**:
```go
// tools/types.go
type Tool interface {
    Name() string
    Description() string
    Schema() ToolSchema
    Execute(ctx context.Context, params json.RawMessage) (Result, error)
}

type Result struct {
    Content string
    Error   error
    Summary string  // 用于 UI 显示摘要
}
```

**工具清单** (Phase 2):
| 工具 | 功能 | 状态 |
|------|------|------|
| `read` | 读取文件内容 | 新建 |
| `write` | 写入/创建文件 | 新建 |
| `edit` | 编辑文件 (查找替换) | 新建 |
| `glob` | 文件模式匹配 | 新建 |
| `grep` | 内容搜索 | 新建 |
| `shell` | 执行 Shell 命令 | 新建 |

**验收标准**:
- [ ] 所有 Phase 2 工具实现完成
- [ ] 工具 JSON Schema 生成
- [ ] 工具执行错误处理
- [ ] 工具结果格式化
- [ ] 单元测试覆盖率 > 70%

---

### 2.2 Agent Loop 重构

**目标**: 实现完整的 ReAct (Reasoning + Acting) 循环

**修改文件**:
```
agent/loop/
├── engine.go        # 重写：实现 ReAct 循环
├── types.go         # 扩展：添加 ToolCall, ToolResult 等类型
├── planner.go       # 新建：任务规划
└── executor.go      # 新建：循环执行器
```

**ReAct 循环流程**:
```
1. 接收 Task
2. 构建 System Prompt + 可用工具描述
3. LOOP:
   a. 调用 LLM 获取响应
   b. 解析响应:
      - 如果是 Thought → 显示给用户
      - 如果是 ToolCall → 执行工具 → 将结果加入上下文 → 继续
      - 如果是 FinalAnswer → 返回结果 → 结束
   c. 检查循环次数/Token 限制
4. 返回结果
```

**Engine 实现**:
```go
// agent/loop/engine.go
func (e *Engine) Run(task Task) ([]Event, error) {
    ctx := &executionContext{
        task:        task,
        messages:    []llm.Message{e.buildSystemPrompt()},
        tools:       e.toolRegistry.List(),
        maxIter:     e.config.MaxIterations,
        events:      []Event{},
    }
    
    for i := 0; i < ctx.maxIter; i++ {
        // 调用 LLM
        resp, err := e.provider.Complete(ctx, ctx.messages, ctx.tools)
        if err != nil {
            return nil, err
        }
        
        // 处理响应
        if resp.FinishReason == "tool_calls" {
            // 执行工具
            for _, tc := range resp.ToolCalls {
                result := e.executeTool(ctx, tc)
                ctx.addToolResult(tc, result)
            }
        } else {
            // 最终答案
            ctx.addEvent(Event{Type: "result", Message: resp.Content})
            break
        }
    }
    
    return ctx.events, nil
}
```

**验收标准**:
- [ ] 实现完整的 ReAct 循环
- [ ] 支持工具调用解析
- [ ] 支持思考过程显示
- [ ] 循环安全限制 (最大迭代、Token 限制)
- [ ] 错误恢复机制
- [ ] 与现有 Event 系统集成

---

### 2.3 上下文管理实现

**目标**: 实现智能上下文管理

**修改文件**:
```
agent/context/
├── manager.go       # 重写：上下文组装与管理
├── budget.go        # 扩展：Token 预算控制
├── compact.go       # 重写：智能压缩
└── tokenizer.go     # 新建：Token 计数
```

**功能**:
- Token 使用追踪
- 上下文窗口管理
- 智能压缩 (摘要、丢弃旧消息)
- 优先级排序 (System > Recent > Important)

**验收标准**:
- [ ] 准确的 Token 计数
- [ ] 预算超限警告
- [ ] 自动压缩策略
- [ ] 与 Engine 集成

---

## Phase 3: 功能完善 (Week 3-4)

### 3.1 权限控制系统

**目标**: 实现工具调用权限控制

**修改文件**:
```
agent/loop/
├── permission.go    # 重写：权限服务实现
└── policy.go        # 新建：权限策略
```

**权限级别**:
- `auto`: 自动执行 (只读、安全命令)
- `ask`: 每次询问
- `deny`: 禁止执行

**验收标准**:
- [ ] 工具分级权限
- [ ] 用户确认交互
- [ ] 白名单/黑名单支持
- [ ] 配置持久化

---

### 3.2 记忆系统实现

**目标**: 实现跨会话记忆

**修改文件**:
```
agent/memory/
├── store.go         # 重写：文件/SQLite 存储实现
├── retrieve.go      # 重写：语义检索
├── policy.go        # 扩展：保留策略
├── embedding.go     # 新建：向量化接口
└── sqlite.go        # 新建：SQLite 实现
```

**功能**:
- 会话历史存储
- 重要信息提取
- 语义检索 (可选: 集成 Embedding)

**验收标准**:
- [ ] 持久化存储
- [ ] 会话恢复
- [ ] 相关信息检索
- [ ] 自动清理策略

---

### 3.3 执行追踪与报告

**目标**: 完善执行追踪和报告生成

**修改文件**:
```
trace/
├── writer.go        # 重写：结构化追踪
├── file.go          # 新建：文件写入
└── format.go        # 新建：格式化

report/
├── summary.go       # 重写：报告生成
└── template.go      # 新建：模板系统
```

**验收标准**:
- [ ] 执行步骤记录
- [ ] Token 使用统计
- [ ] 时间追踪
- [ ] 报告导出 (Markdown/JSON)

---

## Phase 4: 集成与优化 (Week 4-5)

### 4.1 系统集成

**目标**: 整合所有组件，实现端到端流程

**修改文件**:
```
app/
├── bootstrap.go     # 扩展：完整依赖注入
└── run.go           # 调整：真实模式完善

executor/
└── runner.go        # 重写：真实执行器
```

**验收标准**:
- [ ] 启动流程完整
- [ ] 配置正确加载
- [ ] 所有组件初始化
- [ ] Demo 模式与 Real 模式切换正常

---

### 4.2 测试与质量

**目标**: 建立测试体系，确保代码质量

**新增测试文件**:
```
agent/loop/*_test.go
integrations/llm/*_test.go
tools/*/*_test.go
configs/*_test.go
```

**测试策略**:
- 单元测试: 核心业务逻辑
- 集成测试: Provider 调用、工具执行
- Mock 测试: LLM 响应模拟

**验收标准**:
- [ ] 核心模块测试覆盖率 > 70%
- [ ] 集成测试通过
- [ ] E2E 测试场景定义

---

### 4.3 性能优化

**优化点**:
- 并发工具执行
- 连接池 (HTTP Client)
- 响应缓存
- 内存使用优化

---

## 目录结构变更总览

### 新增目录/文件
```
integrations/llm/           # 新增：LLM Provider 层
├── provider.go
├── config.go
├── registry.go
├── openai/
│   ├── client.go
│   └── config.go
└── openrouter/
    ├── client.go
    └── config.go

tools/
├── registry.go             # 新增：工具注册
├── executor.go             # 新增：工具执行
├── types.go                # 新增：工具类型
├── fs/
│   ├── read.go             # 新增
│   ├── write.go            # 新增
│   ├── edit.go             # 新增
│   └── grep.go             # 新增
└── shell/
    └── runner.go           # 新增

agent/loop/
├── planner.go              # 新增
└── executor.go             # 新增

agent/context/
└── tokenizer.go            # 新增

agent/memory/
├── embedding.go            # 新增
└── sqlite.go               # 新增

configs/
├── loader.go               # 新增
├── types.go                # 新增
└── validator.go            # 新增

trace/
├── file.go                 # 新增
└── format.go               # 新增

report/
└── template.go             # 新增
```

### 修改的文件
```
app/bootstrap.go            # 扩展依赖注入
app/run.go                  # 完善真实模式

agent/loop/engine.go        # 重写 ReAct 循环
agent/loop/types.go         # 扩展类型
agent/loop/permission.go    # 实现权限服务

agent/context/manager.go    # 实现上下文管理
agent/context/budget.go     # 扩展预算控制
agent/context/compact.go    # 实现压缩逻辑

agent/memory/store.go       # 实现存储
agent/memory/retrieve.go    # 实现检索
agent/memory/policy.go      # 扩展策略

tools/fs/fs.go              # 集成工具实现
tools/shell/shell.go        # 集成命令执行

executor/runner.go          # 重写执行器

trace/writer.go             # 实现追踪
report/summary.go           # 实现报告
```

---

## 里程碑计划

| 阶段 | 目标 | 预计时间 | 关键交付物 |
|------|------|----------|------------|
| Phase 1 | 基础架构 | Week 1-2 | Provider 接口、配置系统、OpenAI/OpenRouter 实现 |
| Phase 2 | Agent 核心 | Week 2-3 | 工具系统、ReAct 循环、上下文管理 |
| Phase 3 | 功能完善 | Week 3-4 | 权限控制、记忆系统、追踪报告 |
| Phase 4 | 集成优化 | Week 4-5 | 系统集成、测试覆盖、性能优化 |

---

## 技术决策记录

### ADR 1: LLM Provider 接口设计
- **决策**: 定义统一 Provider 接口，每个 Provider 独立实现
- **理由**: 支持灵活扩展，隔离 Provider 差异
- **替代方案**: 使用第三方统一库 (如 LangChain-go) - 过于重量级

### ADR 2: 工具系统架构
- **决策**: 每个工具实现 Tool 接口，通过 Registry 注册
- **理由**: 符合 Go 接口设计，便于测试和扩展
- **替代方案**: 函数映射表 - 不够灵活

### ADR 3: 上下文压缩策略
- **决策**: 保留 System Prompt + 最近 N 条消息 + 重要消息摘要
- **理由**: 平衡上下文完整性和 Token 限制
- **替代方案**: 全量存储到向量数据库 - 实现复杂度高

### ADR 4: 配置管理
- **决策**: 自研配置加载器，支持 YAML + 环境变量
- **理由**: 依赖少，控制力强，满足项目需求
- **替代方案**: Viper - 依赖较重

---

## 风险与缓解

| 风险 | 可能性 | 影响 | 缓解措施 |
|------|--------|------|----------|
| Provider API 变更 | 中 | 高 | 抽象接口，隔离变化 |
| Token 计算不准确 | 中 | 中 | 使用官方 Tokenizer，预留 Buffer |
| 工具执行安全风险 | 中 | 高 | 权限控制 + 命令白名单 |
| 循环无法收敛 | 低 | 高 | 迭代限制 + Token 限制 + 超时 |
| 性能瓶颈 | 低 | 中 | 性能测试 + 缓存优化 |
