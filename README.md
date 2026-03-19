# ms-cli

AI infrastructure agent

## Documentation Map

Current documentation in [`docs/`](docs/) is split into:

- shared repository policy: [`docs/agent-contributor-guide.md`](docs/agent-contributor-guide.md)
- current architecture references:
  - [`docs/arch.md`](docs/arch.md)
  - [`docs/ms-cli-arch.md`](docs/ms-cli-arch.md)
- active refactor and workstream plans:
  - [`docs/impl-guide/ms-cli-refactor-3.md`](docs/impl-guide/ms-cli-refactor-3.md)
  - [`docs/impl-guide/ms-skills-whole-update-plan.md`](docs/impl-guide/ms-skills-whole-update-plan.md)
  - [`docs/impl-guide/ms-factory-struct-v0.1.md`](docs/impl-guide/ms-factory-struct-v0.1.md)
  - [`docs/features-backlog.md`](docs/features-backlog.md)
  - [`docs/how-to-provide-plan-proposal.md`](docs/how-to-provide-plan-proposal.md)

Important:

- architecture docs describe the current checkout
- refactor/workstream docs describe planned target states
- if they conflict, treat the current code as authoritative

## Prerequisites

- Go 1.24.2+ (see `go.mod`)

## Quick Start

Build:

```bash
go build -o ms-cli ./cmd/ms-cli
```

Run demo mode:

```bash
go run ./cmd/ms-cli --demo
# or
./ms-cli --demo
```

Run real mode:

```bash
go run ./cmd/ms-cli
# or
./ms-cli
```

### Command-Line Options

```bash
# Select URL and model
./ms-cli --url https://api.openai.com/v1 --model gpt-4o

# Use custom config file
./ms-cli --config /path/to/config.yaml

# Set API key directly
./ms-cli --api-key sk-xxx
```

## Repository Structure

See [`docs/arch.md`](docs/arch.md) and [`docs/ms-cli-arch.md`](docs/ms-cli-arch.md)
for the current architecture and package map.

The repository is under active refactor, so this README intentionally does not
duplicate a full package tree. Use the linked architecture docs above as the
source of truth for either:

- the current checkout layout, or
- explicitly labeled target-state planning docs under [`docs/`](docs/)

## Known Limitations

- The real-mode engine flow is still minimal/stub-oriented.
- Running Bubble Tea in non-interactive shells may fail with `/dev/tty` errors.

## Planning Workstreams

The repository currently tracks three related planning streams:

- Workstream A: `ms-cli` refactor into a thinner agent runtime
- Workstream B: `ms-skills` update for prompt-oriented domain skills
- Workstream C: incubating Factory schemas, cards, and pack format

These plans live under [`docs/`](docs/) and are intended to guide staged
implementation across `ms-cli`, `ms-skills`, and the future Factory split.

## Architecture Rule

UI listens to events; agent loop emits events; tool execution does not depend on UI.
