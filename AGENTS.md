# AGENTS.md

Follow the shared repository policy in [`docs/agent-contributor-guide.md`](./docs/agent-contributor-guide.md).

Codex/OpenAI-specific notes:

- Prefer minimal diffs over broad cleanup.
- Validate with the narrowest relevant Go command first, then widen if needed.
- When architecture notes conflict, trust the current code and update the docs.
- For current architecture, read [`docs/arch.md`](./docs/arch.md).
- For design rationale, read docs under [`docs/design/`](./docs/design/).
- For implementation plans, read docs under [`docs/impl-guide/`](./docs/impl-guide/).
- For deferred features, read [`docs/features-backlog.md`](./docs/features-backlog.md).
