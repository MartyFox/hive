# hive — Host Isolated Virtual Environment

A single Go binary that runs AI coding agents in isolated Podman containers.
Ships Claude Code, GitHub Copilot CLI, Gemini CLI, and OpenAI Codex CLI,
each in their own hardened container with read-write access to your project only.

## Why a binary instead of a shell script?

- **Self-contained** — all Containerfiles are embedded via `//go:embed`. No separate `images/` directory needed at runtime.
- **Portable** — `go install` or drop a single file anywhere; no bash version requirements.
- **Auto-provision** — `hive run claude` pulls from the registry on first use. No build step required.

## Requirements

- **macOS**: [Podman Desktop](https://podman-desktop.io) (manages the Linux VM automatically)
- **Linux**: `podman` package from your distro

No Docker. No root. Podman runs rootless by default.

## Install

```bash
go install github.com/martinf/hive@latest
```

Or build from source:

```bash
git clone https://github.com/martinf/hive
cd hive && go build -o hive .
```

## Quick start

```bash
# Navigate to your project
cd ~/my-project

# Run an agent — pulls image automatically on first use
hive run claude
hive run copilot
hive run gemini
hive run codex
```

No `hive build` needed. On first run, hive pulls the pre-built image from
`ghcr.io/martinf/hive-<agent>:latest` and tags it locally. Subsequent runs
use the cached local image.

## Commands

### `hive run <agent>`

Run an agent REPL in the current directory.

```bash
hive run claude               # interactive Claude Code session
hive run copilot              # interactive Copilot CLI session
hive run gemini               # interactive Gemini CLI session
hive run codex                # interactive Codex CLI session

hive run claude --cmd "add input validation to packages/api/src/routes/auth.ts"
```

`--cmd` runs a one-shot non-interactive task and exits. Useful for CI or
scripted orchestration.

**Image resolution order:**
1. Local image `hive-<agent>` exists → use it
2. Pull `ghcr.io/martinf/hive-<agent>:latest` → tag locally → use it
3. Pull failed (offline / registry unavailable) → build from embedded Containerfiles

### `hive build [agent|base|all]`

Build images locally from the Containerfiles embedded in the binary.
Use this when you want full control, are offline, or want to customise the images.

```bash
hive build            # build all (base first, then all agents)
hive build claude     # build claude image only
hive build base       # build the shared base image only
```

### `hive update [agent|base|all]`

Rebuild without cache — forces `npm install` to fetch the latest published
CLI versions.

```bash
hive update           # update all images
hive update copilot   # update copilot only
```

### `hive list`

Show locally available hive images with size and age.

```bash
hive list
```

## Agents and YOLO mode

All agents start in full-autonomy / approve-all mode so they can work
without interactive permission prompts:

| Agent | Binary | Flag |
|---|---|---|
| Claude Code | `claude` | `--dangerously-skip-permissions` |
| GitHub Copilot CLI | `copilot` | `--yolo` |
| Google Gemini CLI | `gemini` | *(default approve-all)* |
| OpenAI Codex CLI | `codex` | *(default approval mode)* |

## Global config — auth and personal instructions

Each agent mounts its config directory from the host read-write into the
container. This persists login credentials and personal instructions across
sessions without any extra setup.

| Agent | Host path | Container path |
|---|---|---|
| claude | `~/.claude/` | `/home/agent/.claude/` |
| copilot | `~/.copilot/` | `/home/agent/.copilot/` |
| gemini | `~/.gemini/` | `/home/agent/.gemini/` |
| codex | `~/.config/openai/` | `/home/agent/.config/openai/` |

`copilot` respects the `COPILOT_HOME` environment variable if set.

If the host directory does not exist, hive warns and starts without it.
After logging in inside the container, the CLI creates the directory
automatically and future sessions will mount it.

**Personal instructions** live inside these same directories — drop your
instruction files there as normal and they are picked up automatically:

- Claude: `~/.claude/CLAUDE.md`
- Copilot: `~/.copilot/` (agents/, settings.json, mcp-config.json, `*.instructions.md`)
- Gemini: `~/.gemini/GEMINI.md`

## Project instructions

Project-level instructions live in the workspace and are picked up
automatically when the agent starts in `/workspace`:

| File | Read by |
|---|---|
| `CLAUDE.md` | Claude Code |
| `AGENTS.md` | Claude Code, Copilot CLI |
| `.github/copilot-instructions.md` | Copilot CLI |
| `.github/instructions/**/*.instructions.md` | Copilot CLI |
| `GEMINI.md` | Gemini CLI |

## Security model

| Control | Value |
|---|---|
| Linux capabilities | `--cap-drop=ALL` |
| Privilege escalation | `--security-opt no-new-privileges` |
| Network | Isolated bridge `hive-net`; internet allowed, host LAN isolated via Podman Machine VM boundary (macOS) |
| Container filesystem | Ephemeral (`--rm`); only bind-mounted paths persist |
| User inside container | `agent` (uid 1000, non-root) |

## Workspace — file access

`$PWD` is bind-mounted read-write at `/workspace`. The agent writes directly
to your project files — same inodes, no copying, changes visible instantly on
the host. When the container exits, the filesystem is discarded; only your
project files and the config bind mounts persist.

## Beads (bd) — issue tracking

If the workspace has no `.beads/` directory, hive runs `bd init` before
handing off to the agent. This gives the agent a local issue tracker from
the first session.

## Project structure

```
hiveGo/
├── main.go                          entry point
├── go.mod                           module github.com/martinf/hive
├── cmd/
│   ├── root.go                      cobra root command
│   ├── build.go                     hive build — extract embedded Containerfiles, podman build
│   ├── run.go                       hive run  — image resolution + syscall.Exec for TTY handoff
│   ├── update.go                    hive update — --no-cache rebuild
│   └── list.go                      hive list  — podman images
└── internal/
    ├── podman/podman.go             podman helpers: check, build, pull, tag, network, mount logic
    └── imgfs/
        ├── imgfs.go                 //go:embed images
        └── images/                  Containerfiles baked into binary
            ├── base/Containerfile   node:22-bookworm-slim + git/curl/jq/ripgrep/zsh/python3/bd
            ├── claude/Containerfile @anthropic-ai/claude-code
            ├── copilot/Containerfile @github/copilot
            ├── gemini/Containerfile @google/gemini-cli
            └── codex/Containerfile  @openai/codex
```

## Podman Machine — macOS notes

On macOS, Podman runs inside a Linux VM (Podman Machine). hive checks whether
the machine is running and starts it automatically if not.

The Podman Machine also exposes a Docker-compatible socket useful for other
tooling (Testcontainers, CI scripts, etc.):

```bash
export DOCKER_HOST=unix://$XDG_RUNTIME_DIR/podman/podman.sock
```
