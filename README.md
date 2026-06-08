<p align="center">
  <img src="https://raw.githubusercontent.com/MartyFox/hive/refs/heads/main/docs/brand/logo-lockup-light.png" alt="hive - Host Isolated Virtual Environment" width="720"/>
</p>

<p align="center">
  <a href="https://github.com/MartyFox/hive/actions/workflows/build-images.yml"><img src="https://github.com/MartyFox/hive/actions/workflows/build-images.yml/badge.svg" alt="Build images"></a>
  <a href="https://github.com/MartyFox/hive/actions/workflows/release-binary.yml"><img src="https://github.com/MartyFox/hive/actions/workflows/release-binary.yml/badge.svg" alt="Release"></a>
  <!-- <a href="https://github.com/MartyFox/hive/releases/latest"><img src="https://img.shields.io/github/v/release/MartyFox/hive" alt="Latest release"></a> -->
  <!-- <img src="https://img.shields.io/badge/go-1.21+-00ADD8?logo=go&logoColor=white" alt="Go 1.21+"> -->
  <!-- <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue" alt="Apache 2.0"></a> -->
</p>

<p align="center">
A single Go binary that runs AI coding agents in isolated Podman containers.<br>
Ships Claude Code, GitHub Copilot CLI, Gemini CLI, and OpenAI Codex CLI —<br>
each in its own hardened container with read-write access to your project workspace.
</p>

---

## Table of Contents

- [Requirements](#requirements)
- [Install](#install)
  - [Option 1: `go install`](#option-1-go-install)
  - [Option 2: build from source](#option-2-build-from-source)
  - [Option 3: download a release binary](#option-3-download-a-release-binary)
- [Quick Start](#quick-start)
- [Commands](#commands)
  - [`hive run <agent>`](#hive-run-agent)
  - [`hive build [agent|base|all]`](#hive-build-agentbaseall)
  - [`hive update [agent|base|all]`](#hive-update-agentbaseall)
  - [`hive list`](#hive-list)
  - [`hive version`](#hive-version)
- [Agents and Approval Mode](#agents-and-approval-mode)
- [Global Config — Auth and Personal Instructions](#global-config--auth-and-personal-instructions)
  - [Authentication](#authentication)
- [Project Instructions](#project-instructions)
- [Configuration](#configuration)
  - [Example `~/.hive/config.yaml`](#example-hiveconfigyaml)
  - [Supported Keys](#supported-keys)
  - [Example Legacy `~/.hive/config`](#example-legacy-hiveconfig)
- [Corporate Proxy / TLS Interception](#corporate-proxy--tls-interception)
- [Images](#images)
  - [How Image Resolution Works](#how-image-resolution-works)
  - [Supplying Custom Images](#supplying-custom-images)
- [Security Model](#security-model)
- [Workspace](#workspace)
- [Beads (`bd`) — Issue Tracking](#beads-bd--issue-tracking)
- [Podman Machine — macOS Notes](#podman-machine--macos-notes)
- [Contributing](#contributing)
- [License](#license)

---

## Requirements

- **macOS**: [Podman Desktop](https://podman-desktop.io)
- **Linux**: `podman` for your distro

No Docker daemon. No root. Podman runs rootless by default.

## Install

### Option 1: `go install`

```bash
go install github.com/MartyFox/hive@latest
```

### Option 2: build from source

```bash
git clone https://github.com/MartyFox/hive
cd hive
go build -o hive .
```

### Option 3: download a release binary

Download the correct binary for your platform from [GitHub Releases](https://github.com/MartyFox/hive/releases/latest), make it executable, then place it on your `PATH`.

```bash
chmod +x hive_darwin_arm64
mv hive_darwin_arm64 /usr/local/bin/hive
```

## Quick Start

```bash
hive run claude
hive run copilot
hive run gemini
hive run codex
```

## Commands

### `hive run <agent>`

Run an agent in the current directory.

```bash
# interactive REPL
hive run claude

# run a one-shot shell command in the isolated sandbox
hive run claude --cmd "npm test -- --testPathPattern=auth"
hive run copilot --cmd "find src -name '*.test.ts' | head -5"

# pass a prompt directly (claude and copilot only)
hive run claude --prompt "write unit tests for src/utils/parser.ts"
hive run copilot --prompt "open a PR for these changes"
```

| Flag | Description |
|---|---|
| `--cmd <shell>` | Run a one-shot shell command instead of interactive REPL. Passed verbatim to `bash -c` — only pass trusted strings; `$(...)` executes. |
| `--prompt <text>` | Pass a prompt to the agent non-interactively. Supported by `claude` and `copilot` only — other agents return an error. Prompt text is never evaluated by the shell. |
| `--writable-config` | Mount host agent config read-write. Use only for first-run login or setup flows that must update host config. |
| `--gh-token` | Inject host `gh auth token` into the container via a temporary Podman secret. |

### `hive build [agent|base|all]`

Build images locally from embedded Containerfiles. If no argument is given, builds base then all agents. Building an individual agent image automatically builds the base image first if it is not present.

```bash
hive build
hive build claude
hive build base
```

### `hive update [agent|base|all]`

Rebuild without cache to pick up the latest published CLI versions.

```bash
hive update
hive update copilot
```

### `hive list`

```bash
hive list
```

### `hive version`

```bash
hive version
```

## Agents and Approval Mode

All agents start in high-autonomy mode:

| Agent | Binary | Startup flag |
|---|---|---|
| Claude Code | `claude` | `--dangerously-skip-permissions` |
| GitHub Copilot CLI | `copilot` | `--yolo` |
| Google Gemini CLI | `gemini` | *(CLI default)* |
| OpenAI Codex CLI | `codex` | *(CLI default)* |

## Global Config — Auth and Personal Instructions

Host agent config mounts read-only by default — agents can read credentials, skills, and personal instructions without modifying the host copy. Writable agent state (session files, caches) lives at `~/.hive/state/<agent>/` on the host, mounted read-write at `/home/agent/.hive-state/` inside the container.

Use `--writable-config` or `HIVE_AGENT_CONFIG_MODE=writable` only for login or setup flows that must update the host config.

| Agent | Default host path | Container path | Default mode | Override key |
|---|---|---|---|---|
| claude | `~/.claude/` | `/home/agent/.claude/` | `ro` | `CLAUDE_HOME` |
| copilot | `~/.copilot/` | `/home/agent/.copilot/` | `ro` | `COPILOT_HOME` |
| gemini | `~/.gemini/` | `/home/agent/.gemini/` | `ro` | `GEMINI_HOME` |
| codex | `~/.config/openai/` | `/home/agent/.config/openai/` | `ro` | `CODEX_HOME` |
| all | `~/.agents/` | `/home/agent/.agents/` | `ro` | `AGENTS_HOME` |
| all | `~/.hive/state/<agent>/` | `/home/agent/.hive-state/` | `rw` | *(not configurable)* |

Override keys (`CLAUDE_HOME`, `COPILOT_HOME`, etc.) accept absolute paths or paths starting with `~`. Paths that expose broad credential stores — such as `~/.ssh`, `~/.gnupg`, `~/.aws`, `~/.config/gcloud`, or `~/.kube` — are rejected to prevent accidental host exposure. Use a more specific subdirectory.

If a host directory does not exist, hive warns and starts without it.

### Authentication

- **Claude**: prompts for login on first start
- **Copilot**: type `/login` if not authenticated
- **Gemini**: prompts for login on first start
- **Codex**: prompts for API key or login on first start

Use `--gh-token` to inject the host `gh auth token` into the container as `GH_TOKEN` (Copilot also receives `GITHUB_PERSONAL_ACCESS_TOKEN`). The token is passed via a temporary Podman secret — not baked into images. Set `HIVE_GH_TOKEN_MODE=env-file` if Podman secret support is unavailable. Use a least-privilege token.

Personal instructions live in the mounted config dirs:

- Claude: `~/.claude/CLAUDE.md`
- Copilot: `~/.copilot/` (`agents/`, `settings.json`, `*.instructions.md`, project instructions)
- Gemini: `~/.gemini/GEMINI.md`

## Project Instructions

| File | Read by |
|---|---|
| `CLAUDE.md` | Claude Code |
| `AGENTS.md` | Claude Code, Copilot CLI |
| `.github/copilot-instructions.md` | Copilot CLI |
| `.github/instructions/**/*.instructions.md` | Copilot CLI |
| `GEMINI.md` | Gemini CLI |

## Configuration

hive reads `~/.hive/config.yaml`. Falls back to legacy `~/.hive/config` (`KEY=VALUE`); env vars take highest precedence.

```bash
mkdir -p ~/.hive && touch ~/.hive/config.yaml
```

### Example `~/.hive/config.yaml`

```yaml
network: hive-net
registry: ghcr.io/martyfox
tlsVerify: true

github:
  tokenMode: off # off | podman-secret | env-file

agentConfig:
  mode: read-only # read-only | writable
  paths:
    claude: ~/.claude
    copilot: ~/.copilot
    gemini: ~/.gemini
    codex: ~/.config/openai
    agents: ~/.agents

mounts:
  - name: project-docs
    host: ~/Documents/project-docs
    container: /mnt/project-docs
    mode: read-only # read-only | writable
```

**Extra mount constraints:**

- `host` must be an absolute path or start with `~`. Shell variables (`$VAR`) are rejected.
- `container` must be under `/mnt/`.
- Paths inside sensitive credential directories are rejected by default. Add `allowDangerousHostPath: true` to the mount entry to override:

```yaml
mounts:
  - name: corp-certs
    host: ~/.corp/certs
    container: /mnt/certs
    mode: read-only
    allowDangerousHostPath: true
```

### Supported Keys

| Key | Default | Description |
|---|---|---|
| `HIVE_NETWORK` | `hive-net` | Podman bridge network name |
| `HIVE_REGISTRY` | `ghcr.io/martyfox` | Registry base URL for image pulls |
| `HIVE_TLS_VERIFY` | *(unset)* | Set to `false` to disable TLS verification for Podman pull/build |
| `HIVE_AGENT_CONFIG_MODE` | `read-only` | Set to `writable` or `rw` to mount host agent config read-write |
| `HIVE_GH_TOKEN_MODE` | `off` | Set to `podman-secret` or `env-file` to inject host `gh` token; `true`/`1` map to `env-file` |
| `HIVE_BEADS` | *(unset)* | Set to `1` to install `bd` in base image and auto-run `bd init` before `--cmd` tasks |
| `HIVE_BEADS_VERSION` | `1.0.4` | Pinned `@beads/bd` version used when `HIVE_BEADS=1` |
| `CLAUDE_HOME` | `~/.claude` | Host path mounted as Claude config |
| `COPILOT_HOME` | `~/.copilot` | Host path mounted as Copilot config |
| `GEMINI_HOME` | `~/.gemini` | Host path mounted as Gemini config |
| `CODEX_HOME` | `~/.config/openai` | Host path mounted as Codex config |
| `AGENTS_HOME` | `~/.agents` | Shared skills/agents directory mounted into all containers |

### Example Legacy `~/.hive/config`

```ini
HIVE_REGISTRY=ghcr.io/my-org
HIVE_TLS_VERIFY=false
CLAUDE_HOME=/Volumes/external/.claude
HIVE_GH_TOKEN_MODE=podman-secret
HIVE_BEADS=1
HIVE_BEADS_VERSION=1.0.4
```

## Corporate Proxy / TLS Interception

Not behind a corporate proxy? Skip this section.

If behind TLS interception:
1. Export the proxy root certificate as PEM to `~/.hive/extra-ca.pem`
2. Optionally set `HIVE_TLS_VERIFY=false` if Podman pull/build still fails
3. Build images locally with `hive build`

```bash
# macOS
security find-certificate -a -p /Library/Keychains/System.keychain > ~/.hive/extra-ca.pem
```

`extra-ca.pem` is optional. At runtime hive bind-mounts it and sets `NODE_EXTRA_CA_CERTS` so Node.js CLIs trust your proxy. Do not publish locally built images that contain your CA — GHCR images are always built clean.

## Images

### How Image Resolution Works

```text
hive run copilot
  ├─ local image hive-copilot exists?         → use it
  ├─ pull <HIVE_REGISTRY>/hive-copilot:latest → tag + use it
  └─ pull failed                              → build locally from embedded Containerfiles
```

### Supplying Custom Images

```bash
podman tag my-custom-copilot:latest hive-copilot
hive run copilot
```

Security controls are applied by `hive run`, not baked into the image.

## Security Model

| Control | Value |
|---|---|
| Linux capabilities | `--cap-drop=ALL` |
| Privilege escalation | `--security-opt no-new-privileges` |
| Network | Isolated bridge `hive-net`; internet allowed |
| Container filesystem | Ephemeral (`--rm`) except bind mounts |
| User inside container | `agent` (uid 1000, non-root) |
| Host agent config | Read-only by default; explicit writable mode available |
| GitHub auth injection | Off by default; temporary Podman secret, env-file fallback |

## Workspace

`$PWD` is bind-mounted read-write at `/workspace`. Agents edit real project files directly. Hive-managed state persists under `~/.hive/state/<agent>/`.

## Beads (`bd`) — Issue Tracking

[Beads](https://github.com/gastownhall/beads/tree/main) is an optional local issue tracker. Set `HIVE_BEADS=1` to install `bd` in the base image and auto-run `bd init` before `--cmd` tasks.

## Podman Machine — macOS Notes

On macOS, Podman runs inside a Linux VM. hive detects and starts the Podman Machine automatically when needed. If the machine has not been initialised yet, run:

```bash
podman machine init
podman machine start
```

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for development expectations and contribution licensing. Do not report vulnerabilities in public issues — see [`SECURITY.md`](SECURITY.md).

## License

Apache License, Version 2.0. See [`LICENSE`](LICENSE) and [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).
