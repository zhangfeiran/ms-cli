# ms-cli Architecture

## 1. 目标与边界

本文档仅描述 `ms-cli` 当前代码中的真实架构实现与运行链路，不承载版本规划或 roadmap 目标。

边界约束如下：

1. 只覆盖仓库当前主线代码行为（`app/`、`agent/`、`tools/` 等）。
2. 不定义未来版本承诺；演进路线由 roadmap 文档维护。
3. 任何架构结论以代码为准，本文档用于帮助实现者快速定位模块关系与扩展入口。

## 2. 系统分层与模块职责

`ms-cli` 当前可按以下分层理解：

1. `app`：CLI/TUI 入口与运行时装配层。负责解析命令（`run/resume/sessions list`）、初始化依赖、桥接 UI 与 Agent。
2. `agent`：核心智能执行层。
3. `tools`：可执行工具层（读写文件、搜索、shell），由 Agent 通过统一 schema 调用。
4. `integrations`：外部模型接入层（OpenAI/Anthropic 协议）。
5. `permission`：工具调用授权层（工具级、命令级、路径级策略）。
6. `ui`：Bubble Tea 终端界面层，消费 `loop.Event` 映射后的 UI 事件。
7. `configs`：配置加载与优先级合并（配置文件、环境变量、命令行）。

补充说明：

1. `executor` 目录仅保留兼容用途，真实执行由 `agent/loop.Engine` 完成。
2. 当前运行链路中，`session` 已接入；`memory` 仍未接入主执行路径。

## 3. 运行时装配（Bootstrap）

入口文件：

1. `app/main.go`
2. `app/cli.go`
3. `app/bootstrap.go`
4. `app/wire.go`

### Demo 路径（`--demo`）

1. 在 `app/bootstrap.go` 初始化配置与 `session.Manager`。
2. 创建 `loop.Engine`（stub provider）。
3. 注入到 `Application`，由 `app/run.go` 的 `runDemo()` 驱动虚拟事件流。

### Real 路径（默认）

`app/bootstrap.go` 负责关键依赖注入：

1. Provider：`initProvider` 根据协议创建 OpenAI/Anthropic 客户端。
2. Tool Registry：`initTools` 注册 `read/write/edit/grep/glob/shell`。
3. Context Manager：创建 `context.Manager` 管理短期上下文。
4. Session Manager：创建 `session.Manager` 管理会话持久化。
5. Permission Service：`permission.NewDefaultPermissionService`。
6. Trajectory Writer：`session.NewSessionTraceWriter`，按 session 固定路径写 JSONL。
7. Engine：`loop.NewEngine` 创建核心执行器。

随后在 `app/wire.go` 的 `attachEngineHooks` 统一注入：

1. `Engine.SetContextManager(...)`
2. `Engine.SetPermissionService(...)`
3. `Engine.SetTraceWriter(...)`
4. `Engine.SetMessageSink(...)`（把会话消息实时写入 `session.Manager`）

## 4. 核心执行时序（Run）

核心步骤流（Real 模式）：

1. 用户在 TUI 输入任务（`app/run.go`）。
2. `Application.runTask` 调用 `Engine.Run(task)`。
3. `loop.Engine` 将用户消息写入 `context.Manager`（短期上下文）。
4. `context.Manager.GetMessages()` 提供当前消息窗口给 provider。
5. Provider 生成回复：
6. 若包含 tool call，`loop.Engine` 先通过 `permission.PermissionService` 授权。
7. 授权通过后调用 `tools.Registry` 中对应工具执行，结果回灌到 `context.Manager`。
8. 每轮关键事件写入会话轨迹（`run_started`、`llm_request`、`tool_result`、`run_finished` 等）。
9. 同时通过 `MessageSink` 把 user/assistant/tool 消息持久化进当前 session。
10. `loop.Event` 回传 `app/run.go`，映射为 UI 事件并展示。

简化链路可表示为：

`用户输入 -> loop.Engine -> context -> provider -> tools -> permission -> session(trajectory) -> UI`

## 5. 会话恢复时序（Resume）

命令入口：`ms-cli resume <session-id>`

恢复流程：

1. `app/cli.go` 解析 `resume` 并写入 `BootstrapConfig.ResumeSessionID`。
2. `app/bootstrap.go` 通过 `sessionManager.Load(id)` 加载会话 JSON。
3. 应用 runtime 快照：
4. `applyModelSnapshot(...)` 恢复协议、模型、URL 等模型配置。
5. `applyPermissionSnapshot(...)` 恢复工具/命令/路径权限策略。
6. `ctxManager.ReplaceMessages(currentSession.Messages)` 恢复上下文消息。
7. `sessionMessagesToUI(...)` 把历史消息转换为 UI 初始消息。
8. `session.NewSessionTraceWriter(sessionStorePath, currentSession.ID)` 复用同一会话轨迹文件。
9. `sessionManager.SetCurrentTracePath(...)` 与 `syncSessionRuntime()` 回写当前 runtime 状态。
10. 进入 `runReal()` 后，后续消息继续走同一套 context/session 持久化链路（含 trajectory）。

## 6. Agent 子系统内部分工

`agent` 目前包含 `loop/context/plan/session/memory` 五个子系统：

1. `agent/loop`：主执行引擎。负责 ReAct 循环、tool 调用、计划模式、事件发射与轨迹写入调用。
2. `agent/context`：短期上下文。负责消息保存、token 估算、预算与压缩策略。
3. `agent/plan`：计划生成与执行。包括 `Planner`（生成）与 `PlanExecutor`（执行）。
4. `agent/session`：会话持久化与恢复。负责 session JSON、runtime 快照、会话轨迹写盘。
5. `agent/memory`：长期记忆模块（策略、检索、SQLite 存储）。当前未接入主执行链路。

关键关系：

1. `loop -> context`：每轮请求都依赖 context。
2. `loop -> plan`：Plan/Review 模式下调度计划子系统。
3. `app -> session`：通过 message sink 与 resume 机制接入会话层。
4. `memory` 目前与运行时链路解耦，仅在测试中使用。

## 7. 状态与持久化

当前状态分层：

1. 短期态（对话窗口）：`context.Manager` 内存中维护消息、预算、压缩统计。
2. 会话长期态：`session.Manager` 持久化完整消息与 runtime 快照。
3. 会话轨迹态：由 `session.EventWriter` 按 session 输出 JSONL 轨迹，记录运行事件。

落盘形式：

1. Session JSON：`.mscli/sessions/<session-id>.json`
2. Trace JSONL：`.mscli/sessions/<session-id>.trajectory.jsonl`

边界原则：

1. `context` 优先服务当前推理窗口与 token 预算控制。
2. `session` 提供跨进程、跨重启恢复能力。
3. trajectory 面向可观测性，不直接作为推理上下文来源。

## 8. 扩展点与演进接口

新增/扩展能力的主要入口如下：

1. 新增 Provider：
2. 在 `integrations/llm/<provider>/` 实现 `llm.Provider`。
3. 在 `app/bootstrap.go:initProvider` 增加协议分支与配置映射。
4. 新增 Tool：
5. 实现 `tools.Tool` 接口（`tools/types.go`）。
6. 在 `app/bootstrap.go:initTools` 注册到 `tools.Registry`。
7. 新增 Run Mode：
8. 在 `agent/plan/mode.go` 扩展 `RunMode`，并在 `agent/loop/engine.go` 增加分支执行逻辑。
9. 新增 Permission 策略：
10. 扩展 `permission.DefaultPermissionService` 的策略检查与快照同步逻辑。
11. 接入 Memory（未来）：
12. 可在 `app/wire.go` 或 `loop.Engine` 的消息生命周期中接入 `memory.Manager`（保存/检索策略需明确）。

## 9. 当前限制与已知风险

以下风险来自当前代码与 `docs/agent-review.md` 的归纳：

1. `context.Manager.shouldCompactLocked` 的预算分支阈值比较存在逻辑问题，可能导致自动压缩触发不稳定。
2. `plan.GeneratePlanPrompt(goal, tools)` 中 `tools` 参数未真正用于 prompt 生成，工具列表仍硬编码。
3. `plan.ExecutionConfig` 的 `MaxRetries`、`TimeoutPerStep` 未完全落到执行路径。
4. `memory.Query.Metadata` 在 `SQLiteStore.Query` 未实现对应过滤。
5. `memory.Retriever` 中 policy 字段落地较弱，检索策略主要仍依赖启发式打分。
6. `/compact` 命令在 `app/commands.go` 仍是提示性实现，未直接调用 `context.Manager.Compact()`。

## 10. 相关文档索引

1. 项目总览：[`README.md`](../README.md)
2. agent 代码评审：[`docs/agent-review.md`](./agent-review.md)
3. roadmap 文档：[`docs/roadmap/ROADMAP.md`](./roadmap/ROADMAP.md)

说明：演进路线与阶段目标在 roadmap 文档中维护，本文仅维护“当前实现架构事实”。
