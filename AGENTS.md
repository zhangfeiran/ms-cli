# AGENTS 协作约束

本文件定义本仓库的工程协作硬约束。凡在本仓库执行任务（包括代码、文档、测试、重构）均需遵守以下规则。

## 强制规则

1. 开工前必须先阅读 `docs/ARCHITECTURE.md`。
2. 每次任务的第一条工作说明必须显式确认“已对照当前 architecture”。
3. 若需求与 architecture 冲突，必须先指出冲突点与受影响模块，再执行实现。
4. 若实现导致架构变化，必须在同次提交里同步更新 `docs/ARCHITECTURE.md`。
5. 若仅做局部修复且不影响架构，需在说明中明确标注“无架构变更”。
6. 禁止在未阅读 architecture 的情况下直接修改 `agent/`、`app/`、`tools/` 核心流程文件。

## 开工检查清单（Checklist）

1. [ ] 已阅读 `docs/ARCHITECTURE.md`
2. [ ] 已定位改动模块在架构中的位置与上下游关系
3. [ ] 已判断本次改动是否需要同步更新 `docs/ARCHITECTURE.md`
