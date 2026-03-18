# ms-cli

AI infrastructure agent

## Documentation Map

Current documentation in [`docs/`](docs/) is split into:

- shared repository policy: [`docs/ai/contributor-guide.md`](docs/ai/contributor-guide.md)
- current architecture references:
  - [`docs/arch.md`](docs/arch.md)
  - [`docs/ms-cli-arch.md`](docs/ms-cli-arch.md)
- active refactor and workstream plans:
  - [`docs/ms-cli-refactor.md`](docs/ms-cli-refactor.md)
  - [`docs/ms-skills-update-plan.md`](docs/ms-skills-update-plan.md)
  - [`docs/incubating-factory-plan.md`](docs/incubating-factory-plan.md)
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

## Commands

In TUI input, use slash commands:

### Project Commands
- `/roadmap status [path]` (default: `roadmap.yaml`)
- `/weekly status [path]` (default: `weekly.md`)

### Model Commands
- `/model` - Show current model configuration
- `/model <model-name>` - Switch to a new model
- `/model <openai:model>` - Backward-compatible provider prefix format (e.g., `/model openai:gpt-4o-mini`)

### Session Commands
- `/compact` - Compact conversation context to save tokens
- `/clear` - Clear chat history
- `/mouse [on|off|toggle|status]` - Control mouse wheel scrolling
- `/exit` - Exit the application
- `/help` - Show available commands

Any non-slash input is treated as a normal task prompt and routed to the engine.

### Slash Command Autocomplete

Type `/` to see available slash commands. Use `↑`/`↓` keys to navigate and `Tab` or `Enter` to select.

## Keybindings

| Key | Action |
|-----|--------|
| `enter` | Send input |
| `mouse wheel` | Scroll chat |
| `pgup` / `pgdn` | Scroll chat |
| `up` / `down` | Scroll chat / Navigate slash suggestions |
| `home` / `end` | Jump to top / bottom |
| `tab` / `enter` | Accept slash suggestion |
| `esc` | Cancel slash suggestions |
| `/` | Start a slash command |
| `ctrl+c` | Quit |

## Project Status Data

Roadmap status engine:

- `internal/project/roadmap.go`
- Parses roadmap YAML, validates schema, and computes phase + overall progress.

Weekly update parser (Markdown + YAML front matter):

- `internal/project/weekly.go`
- Template: `docs/updates/WEEKLY_TEMPLATE.md`

Public roadmap page:

- `docs/roadmap/ROADMAP.md`

Project reports:

- `docs/updates/` (see latest `*-report.md`)

## Repository Structure

See [`docs/arch.md`](docs/arch.md) and [`docs/ms-cli-arch.md`](docs/ms-cli-arch.md)
for the current architecture and package map.

The repository is under active refactor, so this README intentionally does not
duplicate a full package tree. Use the linked architecture docs above as the
source of truth for either:

- the current checkout layout, or
- explicitly labeled target-state planning docs under [`docs/`](docs/)

## Configuration

Configuration can be provided via:

1. **Config file** (`mscli.yaml` or `~/.config/mscli/config.yaml`)
2. **Environment variables**
3. **Command-line flags** (highest priority)

### Environment Variables

| Variable | Description |
|----------|-------------|
| `MSCLI_BASE_URL` | OpenAI-compatible API base URL (higher priority) |
| `MSCLI_MODEL` | Model name |
| `MSCLI_API_KEY` | API key (higher priority) |
| `OPENAI_BASE_URL` | API base URL (fallback) |
| `OPENAI_MODEL` | Model name (fallback) |
| `OPENAI_API_KEY` | API key (fallback) |

### Example Config File

```yaml
model:
  url: https://api.openai.com/v1
  model: gpt-4o-mini
  key: ""
  temperature: 0.7
budget:
  max_tokens: 32768
  max_cost_usd: 10
context:
  max_tokens: 24000
  compaction_threshold: 0.85
```

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
