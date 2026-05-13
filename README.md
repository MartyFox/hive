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
```

One-shot non-interactive task via `--cmd` (exits when complete — useful for CI):

```bash
hive run claude --cmd "add input validation to packages/api/src/routes/auth.ts"
```

One-shot via `--prompt` (same effect, translates to the agent's prompt flag):

```bash
hive run copilot --prompt "refactor the auth module to use async/await"
hive run claude  --prompt "write unit tests for src/utils/parser.ts"
```

**Image resolution order:**
1. Local image `hive-<agent>` exists → use it
2. Pull `<registry>/hive-<agent>:latest` → tag locally → use it
3. Pull failed (offline / registry unavailable) → build from embedded Containerfiles

### `hive build [agent|base|all]`

Build images locally from the Containerfiles embedded in the binary.
Use this when you want full control, are offline, or want to customise the images.

```bash
hive build            # build all (base first, then all agents)
hive build claude     # build claude image only
hive build base       # build the shared base image only
```

`hive build` should create all images which can be viewed using `podman images`

```bash
> podman images
REPOSITORY              TAG               IMAGE ID      CREATED         SIZE
localhost/hive-codex    latest            47e7451891f7  12 minutes ago  892 MB
localhost/hive-gemini   latest            87aaedba87c0  13 minutes ago  831 MB
localhost/hive-copilot  latest            f50f0af0b41a  13 minutes ago  1.1 GB
localhost/hive-claude   latest            8631de8efd5e  15 minutes ago  915 MB
localhost/hive-base     latest            b556092a08af  16 minutes ago  610 MB
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

| Agent | Default host path | Container path | Override key |
|---|---|---|---|
| claude | `~/.claude/` | `/home/agent/.claude/` | `CLAUDE_HOME` |
| copilot | `~/.copilot/` | `/home/agent/.copilot/` | `COPILOT_HOME` |
| gemini | `~/.gemini/` | `/home/agent/.gemini/` | `GEMINI_HOME` |
| codex | `~/.config/openai/` | `/home/agent/.config/openai/` | `CODEX_HOME` |

Host paths can be overridden in `~/.hive/config` (see [Configuration](#configuration)).

If the host directory does not exist, hive warns and starts without it.
After authenticating inside the container, the CLI creates the directory
automatically and future sessions will mount it.

### Authentication
**Claude**, **Gemini** and **Codex** all prompt for login on startup.
**Copilot** does not so you to type `/login` if not authenticated. 
Follow the on-screen device-flow instructions. Credentials are
written to `~/.copilot/` and persisted via bind mount for all future sessions (if chosen).

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

## Configuration

hive reads `~/.hive/config` — a plain `KEY=VALUE` file (shell-style: `#` comments,
no `export`, no quoting needed for simple values). Environment variables always
take precedence over the config file.

Create the directory and file if they do not exist:

```bash
mkdir -p ~/.hive
touch ~/.hive/config
```

### Supported keys

| Key | Default | Description |
|---|---|---|
| `HIVE_NETWORK` | `hive-net` | Podman bridge network name |
| `HIVE_REGISTRY` | `ghcr.io/martinf` | Registry base URL for image pulls |
| `HIVE_TLS_VERIFY` | `true` | Set to `false` to disable TLS verification (corporate proxy) |
| `HIVE_BEADS` | `false` | Set to `1` to install `bd` in the base image and auto-run `bd init` before `--cmd` tasks |
| `CLAUDE_HOME` | `~/.claude` | Host path mounted as Claude config |
| `COPILOT_HOME` | `~/.copilot` | Host path mounted as Copilot config |
| `GEMINI_HOME` | `~/.gemini` | Host path mounted as Gemini config |
| `CODEX_HOME` | `~/.config/openai` | Host path mounted as Codex config |
| `COPILOT_DISABLE_BUILTIN_MCP` | *(unset)* | Set to `1` to suppress Copilot's built-in MCP OAuth (corporate proxy) |

### Example `~/.hive/config`

```ini
# Corporate proxy environment
HIVE_TLS_VERIFY=false
COPILOT_DISABLE_BUILTIN_MCP=1

# Use a team registry instead of the default
HIVE_REGISTRY=ghcr.io/my-org

# Non-standard config locations
CLAUDE_HOME=/Volumes/external/.claude

# Enable beads auto-init before --cmd tasks
HIVE_BEADS=1
```

## Corporate proxy / TLS interception (zScaler)

When a proxy (zScaler, Fiddler, etc.) performs TLS inspection, two problems occur:

1. **`podman pull` / `podman build` fail** — the proxy cert is unknown to the container OS.
2. **Node.js agents fail** — Node.js ships its own CA bundle and ignores the system store.

### Fix

**1. Export the proxy root certificate as PEM:**

```bash
# Export from your system keychain — example on macOS:
security find-certificate -a -p /Library/Keychains/System.keychain > ~/.hive/extra-ca.pem
# Or export only the zScaler root cert from Keychain Access → export as .pem
```

Place the PEM file (may contain multiple certs) at `~/.hive/extra-ca.pem`.

**2. Disable TLS verification for podman pull/build in `~/.hive/config`:**

```ini
HIVE_TLS_VERIFY=false
```

This adds `--tls-verify=false` to all `podman pull` and `podman build` calls.

**3. Rebuild images to bake the cert in:**

```bash
hive build
```

hive copies `~/.hive/extra-ca.pem` directly into the build context so the base
Containerfile can inject it into the OS CA store via `update-ca-certificates`.
At container runtime, hive bind-mounts the cert and sets `NODE_EXTRA_CA_CERTS`
so Node.js agents also trust it.

**4. Disable Copilot MCP OAuth (if MCP server OAuth fails behind proxy):**

```ini
COPILOT_DISABLE_BUILTIN_MCP=1
```

This passes `--disable-builtin-mcps` to the copilot entrypoint, preventing the
`github-mcp-server` OAuth flow that fails when `api.business.githubcopilot.com`
is intercepted.

## Images

### How image resolution works

```
hive run copilot
  ├─ local image hive-copilot exists?         → use it
  ├─ pull <HIVE_REGISTRY>/hive-copilot:latest → tag + use it
  └─ pull failed                              → build from embedded Containerfiles
```

`hive build` always works offline — it is the primary path. The registry is a
convenience layer so users on a fresh machine can skip the build step.

For publishing images to ghcr.io, setting up GitHub Actions CI, or hosting a team
registry, see [docs/registry.md](docs/registry.md).

### Supplying custom images

Users can substitute any image without touching hive source code — just tag it
with the expected local name:

```bash
# Replace the copilot image with a custom build
podman tag my-custom-copilot:latest hive-copilot

# hive picks it up immediately — skips pull+build
hive run copilot
```

The security model (capability drops, network isolation, bind mounts) is applied
by `hive run` at container start, not baked into the image. A custom image still
runs with `--cap-drop=ALL`, `--security-opt no-new-privileges`, and the isolated
network — the image only controls which tools are available inside.

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

Beads (`bd`) is a local, file-based issue tracker. Installation and
auto-initialisation are both **opt-in** — set `HIVE_BEADS=1` in `~/.hive/config`
to install `bd` in the base image and have hive run `bd init` automatically
before `--cmd` tasks when the workspace has no `.beads/` directory.

If you use a different tracker (GitHub Issues, Linear, etc.) leave `HIVE_BEADS`
unset. The `bd` binary is still available inside the container for manual use.

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
            ├── base/Containerfile   node:22-bookworm-slim + git/curl/jq/ripgrep/zsh/python3/gh; bd optional (HIVE_BEADS=1)
            ├── claude/Containerfile @anthropic-ai/claude-code
            ├── copilot/Containerfile @github/copilot + github-mcp-server
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
