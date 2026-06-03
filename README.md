# hive — Host Isolated Virtual Environment

A single Go binary that runs AI coding agents in isolated Podman containers.
Ships Claude Code, GitHub Copilot CLI, Gemini CLI, and OpenAI Codex CLI,
each in its own hardened container with read-write access to your project
workspace and read-only access to host agent config by default.

## Why a binary instead of a shell script?

- **Self-contained** — all Containerfiles are embedded via `//go:embed`.
- **Portable** — install with `go install`, download a release binary, or build from source.
- **Auto-provision** — `hive run <agent>` pulls a prebuilt image on first use.

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

Download the correct binary for your platform from GitHub Releases, make it executable, then place it on your `PATH`.

```bash
chmod +x hive_darwin_arm64
mv hive_darwin_arm64 /usr/local/bin/hive
```

## Quick start

```bash
hive run claude
hive run copilot
hive run gemini
hive run codex
```

`hive build` is optional. `hive run` first tries the local image cache, then pulls `ghcr.io/martyfox/hive-<agent>:latest`, then falls back to building locally.

## Commands

### `hive run <agent>`

Run an agent REPL in the current directory.

```bash
hive run claude
hive run copilot
hive run gemini
hive run codex
```

One-shot task via `--cmd`:

```bash
hive run claude --cmd "add input validation to packages/api/src/routes/auth.ts"
```

> **Note**: `--cmd` is passed verbatim to `bash -c` inside the container.
> Command substitutions like `$(...)` execute. Only pass trusted command
> strings.

Prompt shortcut via `--prompt`:

```bash
hive run copilot --prompt "refactor auth module to use async/await"
hive run claude --prompt "write unit tests for src/utils/parser.ts"
```

Host config mounts are read-only by default. Use `--writable-config` only for
login or setup flows that must update the host agent config directory. GitHub
token injection is off by default; add `--gh-token` for runs that need host
`gh` authentication. The flag uses a temporary Podman secret by default.

Image resolution order:
1. Local image `hive-<agent>` exists
2. Pull `<registry>/hive-<agent>:latest`
3. Pull fails, build locally from embedded Containerfiles

### `hive build [agent|base|all]`

Build images locally from embedded Containerfiles.

```bash
hive build
hive build claude
hive build base
```

### `hive update [agent|base|all]`

Rebuild without cache so `npm install` picks up latest published CLI versions.

```bash
hive update
hive update copilot
```

### `hive list`

Show local hive images with size and age.

```bash
hive list
```

### `hive version`

Show binary version, commit, and build date.

```bash
hive version
```

## Agents and approval mode

All agents start in high-autonomy mode:

| Agent | Binary | Startup flag |
|---|---|---|
| Claude Code | `claude` | `--dangerously-skip-permissions` |
| GitHub Copilot CLI | `copilot` | `--yolo` |
| Google Gemini CLI | `gemini` | *(CLI default)* |
| OpenAI Codex CLI | `codex` | *(CLI default)* |

## Global config — auth and personal instructions

Each agent mounts its host config directory read-only into the container by default. This lets the agent read login state, skills, hooks, MCP definitions, and personal instructions without being able to mutate the host copy. Hive also mounts a writable state directory at `/home/agent/.hive-state` from `~/.hive/state/<agent>/`.

Use `hive run <agent> --writable-config` or set `HIVE_AGENT_CONFIG_MODE=writable` only when an agent must update host config during login or setup.

| Agent | Default host path | Container path | Default mode | Override key |
|---|---|---|---|---|
| claude | `~/.claude/` | `/home/agent/.claude/` | `ro` | `CLAUDE_HOME` |
| copilot | `~/.copilot/` | `/home/agent/.copilot/` | `ro` | `COPILOT_HOME` |
| gemini | `~/.gemini/` | `/home/agent/.gemini/` | `ro` | `GEMINI_HOME` |
| codex | `~/.config/openai/` | `/home/agent/.config/openai/` | `ro` | `CODEX_HOME` |
| all | `~/.agents/` | `/home/agent/.agents/` | `ro` | `AGENTS_HOME` |
| all | `~/.hive/state/<agent>/` | `/home/agent/.hive-state/` | `rw` | *(not configurable)* |

Host paths can be overridden in `~/.hive/config.yaml` or legacy `~/.hive/config`.

If a host directory does not exist, hive warns and starts without it.

### Authentication

- **Claude**: prompts for login on first start
- **Copilot**: type `/login` if not authenticated
- **Gemini**: prompts for login on first start
- **Codex**: prompts for API key or login on first start depending on CLI version

Copilot MCP relies on Copilot CLI's built-in remote SSE MCP transport.

GitHub token injection is opt-in. Use `hive run <agent> --gh-token` when a run needs host `gh` authentication. If `gh auth token` succeeds on the host, hive creates a temporary Podman secret from stdin and exposes it inside the container as `GH_TOKEN`. For Copilot, hive also exposes `GITHUB_PERSONAL_ACCESS_TOKEN` as a compatibility alias for GitHub tooling that expects that variable. Tokens are not baked into images, do not appear in Podman run args, and the temporary Podman secrets are removed after the Podman child process exits.

Set `HIVE_GH_TOKEN_MODE=env-file` only when Podman secret support is not available. Env-file mode writes a temporary `0600` file and passes it with `--env-file`; cleanup runs after the Podman child process exits.

Use a least-privilege GitHub token for agent work where possible. Hive does not reduce the scopes of the token returned by host `gh auth token`.

Personal instructions live in the mounted config dirs:

- Claude: `~/.claude/CLAUDE.md`
- Copilot: `~/.copilot/` (`agents/`, `settings.json`, `*.instructions.md`, project instructions)
- Gemini: `~/.gemini/GEMINI.md`

## Project instructions

Project-level instructions live in the workspace and are picked up automatically when the agent starts in `/workspace`:

| File | Read by |
|---|---|
| `CLAUDE.md` | Claude Code |
| `AGENTS.md` | Claude Code, Copilot CLI |
| `.github/copilot-instructions.md` | Copilot CLI |
| `.github/instructions/**/*.instructions.md` | Copilot CLI |
| `GEMINI.md` | Gemini CLI |

## Configuration

hive reads `~/.hive/config.yaml` for structured runtime policy. It also keeps compatibility with legacy `~/.hive/config` as plain `KEY=VALUE` lines. Environment variables override both files; legacy `~/.hive/config` overrides YAML for matching scalar keys.

```bash
mkdir -p ~/.hive
touch ~/.hive/config.yaml
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

Extra mounts are validated before Podman starts:

- `host` must be absolute or start with `~`; shell variables like `$HOME` are rejected
- `host: /`, the raw home directory, and sensitive config parents are rejected unless `allowDangerousHostPath: true`
- `container` must be under `/mnt/`
- `mode` must be `read-only`/`ro` or `writable`/`rw`

### Supported keys

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

### Example legacy `~/.hive/config`

```ini
# Use a team registry instead of the default
HIVE_REGISTRY=ghcr.io/my-org

# Corporate proxy environment
HIVE_TLS_VERIFY=false

# Non-standard config locations
CLAUDE_HOME=/Volumes/external/.claude

# Allow host gh token injection through Podman secrets
HIVE_GH_TOKEN_MODE=podman-secret

# Enable beads auto-init before --cmd tasks
HIVE_BEADS=1

# Pin beads version for reproducible builds
HIVE_BEADS_VERSION=1.0.4
```

## Corporate proxy / TLS interception

Corporate TLS interception is optional and local-only.

If you are **not** behind a corporate proxy, do nothing.

If you **are** behind TLS interception:
1. Export the proxy root certificate as PEM to `~/.hive/extra-ca.pem`
2. Optionally set `HIVE_TLS_VERIFY=false` if Podman pull/build still fails
3. Build images locally with `hive build`

Example export on macOS:

```bash
security find-certificate -a -p /Library/Keychains/System.keychain > ~/.hive/extra-ca.pem
```

How cert handling works:
- `extra-ca.pem` is **optional**
- public images are built and published **without** your corporate CA
- local corporate builds may include your CA in the locally built base image
- at runtime, hive also bind-mounts `~/.hive/extra-ca.pem` and sets `NODE_EXTRA_CA_CERTS` when that file exists, so Node.js CLIs trust your proxy without changing published images

Security guidance:
- do **not** publish images built with your private corporate CA bundle
- published GHCR images should be built in a clean public environment with no `extra-ca.pem`
- use local builds for corporate environments

## Images

### How image resolution works

```text
hive run copilot
  ├─ local image hive-copilot exists?         → use it
  ├─ pull <HIVE_REGISTRY>/hive-copilot:latest → tag + use it
  └─ pull failed                              → build locally from embedded Containerfiles
```

`hive build` is primary offline path. Registry is convenience layer for fresh machines.

### Supplying custom images

```bash
podman tag my-custom-copilot:latest hive-copilot
hive run copilot
```

Security controls are applied by `hive run`, not baked into the image.

## Security model

| Control | Value |
|---|---|
| Linux capabilities | `--cap-drop=ALL` |
| Privilege escalation | `--security-opt no-new-privileges` |
| Network | Isolated bridge `hive-net`; internet allowed |
| Container filesystem | Ephemeral (`--rm`) except bind mounts |
| User inside container | `agent` (uid 1000, non-root) |
| Host agent config | Read-only by default; explicit writable mode available |
| GitHub auth injection | Off by default; explicit temporary Podman secret, env-file compatibility mode available |

## Workspace — file access

`$PWD` is bind-mounted read-write at `/workspace`. Agents edit your real project files directly. Container filesystem is discarded on exit. Host agent config and shared skills mount read-only by default; Hive-managed state persists under `~/.hive/state/<agent>/`.

## Beads (`bd`) — issue tracking

[Beads](https://github.com/gastownhall/beads/tree/main) is an optional local issue tracking tool. 
Set `HIVE_BEADS=1` to install `bd` in the base image and auto-run `bd init` before `--cmd` tasks when `.beads/` is missing.
Set `HIVE_BEADS_VERSION` in `~/.hive/config` to pin or override the installed `@beads/bd` version.

## Project structure

```text
hive/
├── main.go
├── go.mod                           module github.com/MartyFox/hive
├── cmd/
│   ├── root.go
│   ├── build.go
│   ├── run.go
│   ├── update.go
│   ├── list.go
│   └── version.go
└── internal/
    ├── podman/podman.go
    └── imgfs/
        ├── imgfs.go
        └── images/
            ├── base/Containerfile   node:22-bookworm-slim + git/curl/jq/ripgrep/zsh/python3/gh; bd optional
            ├── claude/Containerfile @anthropic-ai/claude-code
            ├── copilot/Containerfile @github/copilot
            ├── gemini/Containerfile @google/gemini-cli
            └── codex/Containerfile  @openai/codex
```

## Podman Machine — macOS notes

On macOS, Podman runs inside a Linux VM. hive checks whether the machine is running and starts it automatically if needed.

```bash
export DOCKER_HOST=unix://$XDG_RUNTIME_DIR/podman/podman.sock
```

## Contributing

Contributions are welcome. See `CONTRIBUTING.md` for development expectations,
security-sensitive change guidance, and contribution licensing.

Do not report vulnerabilities in public issues. See `SECURITY.md`.

## License

Hive is licensed under the Apache License, Version 2.0. See `LICENSE`.

Third-party dependency notices are listed in `THIRD_PARTY_NOTICES.md`.

Release binaries include `LICENSE`, `NOTICE`, and `THIRD_PARTY_NOTICES.md` as
release assets. Published container images include OCI source and license
labels.
