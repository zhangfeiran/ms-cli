# ms-cli

MindSpore CLI — an AI infrastructure agent with a terminal UI.

## Prerequisites

- Go 1.24.2+ (see `go.mod`)

## Quick Start

Build:

```bash
go build -o ms-cli ./app
```

Run demo mode:

```bash
go run ./app --demo
# or
./ms-cli --demo
```

Run real mode:

```bash
go run ./app
# or
./ms-cli
```

## LLM Configuration

By default, real mode calls an OpenAI-compatible endpoint at `http://localhost:4000/v1`.

Environment variables:

- `OPENAI_API_BASE` (default: `http://localhost:4000/v1`)
- `OPENAI_API_KEY` (optional for local gateways)
- `OPENAI_MODEL` (optional; if empty, the first model from `/models` is used)

Example:

```bash
export OPENAI_API_BASE=http://localhost:4000/v1
export OPENAI_MODEL=qwen2.5-coder-7b-instruct
go run ./app
```

## Runtime Behavior

- Real mode streams agent events while running: each round's `bash` command and command output is shown in the chat panel in real time.
- Every run saves a trajectory JSON file (default: `trace/last-trajectory.json`).

Optional env vars:

- `MSCLI_TRAJECTORY_PATH` (set custom trajectory file path, or set `off` to disable saving)
- `MSCLI_AGENT_STEP_LIMIT` (default: `12`)
- `MSCLI_COMMAND_TIMEOUT_SECONDS` (default: `30`)
- `MSCLI_DEBUG_PROMPT` (default: `true`, print full prompt payload for each round)
- `MSCLI_DEBUG_SHELL_RESULT` (default: `true`, print captured shell result payload each command)
- `MSCLI_TEXT_OBSERVATION_FALLBACK` (default: `false`, append observation as user text for model compatibility)

Observation payload includes `stdout`, `stderr`, `output` and `returncode`.

## Commands

In TUI input, use slash commands:

- `/roadmap status [path]` (default: `roadmap.yaml`)
- `/weekly status [path]` (default: `weekly.md`)

Any non-slash input is treated as a normal task prompt and routed to the engine.

## Keybindings

| Key | Action |
|-----|--------|
| `enter` | Send input |
| `pgup` / `pgdn` | Scroll chat |
| `up` / `down` | Scroll chat |
| `home` / `end` | Jump to top / bottom |
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

```text
ms-cli/
├── app/                        # entry point + wiring
│   ├── main.go
│   ├── bootstrap.go
│   ├── wire.go
│   ├── run.go
│   └── commands.go
├── agent/
│   ├── loop/                   # engine, task/event types, permissions
│   ├── context/                # budget, compaction, context manager
│   └── memory/                 # policy, store, retrieve
├── executor/
│   └── runner.go               # pluggable task executor
├── integrations/
│   ├── domain/                 # external domain client + schema
│   ├── llm/openai/             # OpenAI-compatible LLM client
│   └── skills/                 # skill invocation + repo
├── internal/
│   └── project/
│       ├── roadmap.go
│       └── weekly.go
├── tools/
│   ├── fs/                     # filesystem operations
│   └── shell/                  # shell command runner
├── trace/
│   └── writer.go               # execution trace logging
├── report/
│   └── summary.go              # report generation
├── ui/
│   ├── app.go                  # root Bubble Tea model
│   ├── model/model.go          # shared state types
│   ├── components/             # spinner, textinput, viewport
│   └── panels/                 # topbar, chat, hintbar
├── docs/
│   ├── roadmap/ROADMAP.md
│   └── updates/
├── go.mod
└── README.md
```

## Known Limitations

- The minimal agent currently only supports one tool (`bash`) and no permission/approval gate yet.
- Running Bubble Tea in non-interactive shells may fail with `/dev/tty` errors.

## Architecture Rule

UI listens to events; agent loop emits events; executor/tools do not depend on UI.
