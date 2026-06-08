# Hive — Internal Package API Reference

All packages are internal to the `github.com/MartyFox/hive` module. External consumers cannot import them.

---

## `internal/podman`

Import path: `github.com/MartyFox/hive/internal/podman`

Core package. Contains all Podman integration, config resolution, mount construction, path validation, and token handling.

---

### Constants

```go
const Prefix = "hive"
```

Image name prefix. All local images are named `hive-<agent>`.

---

### Types

#### `RunOptions`

```go
type RunOptions struct {
    Interactive    bool
    WritableConfig bool
    GitHubToken    bool
}
```

Controls host integration for a `podman run` invocation.

| Field | Description |
|---|---|
| `Interactive` | Adds `-it` to run args. Set to `false` for `--cmd` and `--prompt` modes. |
| `WritableConfig` | Mounts agent config dirs `rw` instead of `ro`. Overrides config-level setting. |
| `GitHubToken` | Opts into GitHub token injection when the config-level mode is `off`. Selects `podman-secret` mode. |

---

### Agent Registry

#### `Agents() []string`

Returns a copy of the supported agent name list: `["claude", "copilot", "gemini", "codex"]`. Safe to modify — callers receive a new slice.

#### `ValidAgent(name string) bool`

Reports whether `name` is a supported agent. Case-sensitive.

#### `JoinAgents() string`

Returns a space-prefixed, space-joined string of all agent names: `" claude copilot gemini codex"`. Used to format "valid agents" in error messages.

#### `RegistryName(agent string) string`

Returns the full registry pull reference for an agent: `<registry>/hive-<agent>:latest`. Registry base is resolved from config.

---

### Podman Operations

#### `CheckPodman() error`

Verifies that `podman` is on `$PATH`. On macOS, probes `podman info`; if it fails, runs `podman machine start` automatically. Returns an error if Podman is absent or the machine fails to start.

#### `ImageExists(name string) bool`

Reports whether a local image named `name` exists. Runs `podman image exists <name>`.

#### `PullImage(name string) error`

Runs `podman pull [--tls-verify=false] <name>`, streaming output to stdout/stderr. TLS flag is applied when `HIVE_TLS_VERIFY=false`.

#### `TagImage(src, dst string) error`

Runs `podman tag <src> <dst>`, streaming output to stdout/stderr.

#### `BuildImage(tag, contextDir string, noCache bool, buildArgs []string) error`

Runs `podman build -t <tag> [--no-cache] [--build-arg <k=v>...] <contextDir>`. TLS flag applied when configured. `buildArgs` values are passed as individual `--build-arg` flags.

#### `EnsureNetwork() error`

Creates the configured Podman bridge network if it does not exist. Probes with `podman network inspect`; creates with `--driver bridge --label hive.managed=true` on failure. Network name resolved from config.

---

### Run Argument Assembly

#### `BuildRunArgs(agent string, opts RunOptions) ([]string, func(), error)`

Assembles the full `podman run` argument slice (everything after `podman`, before the image name). Returns the args, a cleanup function (removes temp secrets/env-files), and any error.

The cleanup function is safe to call multiple times — it uses `sync.Once` internally. Callers must run Podman as a child process (not `syscall.Exec`) when the cleanup function is non-trivial.

Argument order: base flags → config mounts → Hive state mount → agent state env → extra mounts → token → cert. `agent state env` is currently Copilot-only and redirects `COPILOT_HOME` into Hive state when host config is read-only.

---

### Config Resolution

#### `Network() string`

Effective Podman bridge network name. Override: `HIVE_NETWORK`, `~/.hive/config`, `network` in YAML. Default: `hive-net`.

#### `AgentConfigWritable() bool`

Reports whether agent config dirs should be mounted `rw`. True when `HIVE_AGENT_CONFIG_MODE` or `agentConfig.mode` is `read-write` or `rw`; legacy `writable` is also accepted.

#### `GitHubTokenEnabled() bool`

Reports whether GitHub token injection is active in config (ignores the `--gh-token` flag). True when the normalised config mode is not `off`.

#### `CopilotHome() string`

Effective Copilot config host path. Override: `COPILOT_HOME`, `agentConfig.paths.copilot` in YAML. Default: `~/.copilot`.

#### `AgentsHome() string`

Effective shared `~/.agents` host path. Override: `AGENTS_HOME`, `agentConfig.paths.agents` in YAML. Default: `~/.agents`.

---

### Image Build Helpers

#### `InjectCertToContext(contextDir string) error`

Copies `~/.hive/extra-ca.pem` into `contextDir` as `extra-ca.pem` when present. No-op when the cert file does not exist. Called before `BuildImage` for the base image only.

#### `BeadsEnabled() bool`

Reports whether the Beads (`bd`) CLI should be included in image builds and auto-run before `--cmd` tasks. True when `HIVE_BEADS=1`.

#### `BeadsArg() string`

Returns the `--build-arg` value for `HIVE_BEADS`: either `"HIVE_BEADS=1"` or `"HIVE_BEADS=0"`.

#### `BeadsVersionArg() string`

Returns the `--build-arg` value for `HIVE_BEADS_VERSION`. Resolved from config with default `1.0.4`. Example: `"HIVE_BEADS_VERSION=1.0.4"`.

---

### Copilot-Specific

#### `CleanCopilotMCPConfig()`

Removes a hive-generated `mcp-config.json` from the Copilot config directory when it contains only the hive-generated `github-mcp-server` entry. Copilot CLI v1.0.44+ uses a remote SSE transport for the built-in GitHub MCP server; leaving a local binary entry causes a name collision.

User-created files with additional MCP server entries are preserved.

---

## `internal/imgfs`

Import path: `github.com/MartyFox/hive/internal/imgfs`

#### `FS embed.FS`

The embedded `images/` directory tree. Contains build contexts for `base`, `claude`, `copilot`, `gemini`, and `codex`.

Access paths use the `images/` prefix:

```go
data, err := imgfs.FS.ReadFile("images/base/Containerfile")
data, err := imgfs.FS.ReadFile("images/claude/Containerfile")
```

Used exclusively by `cmd/build.go`'s `extractBuildContextFromFS` to unpack contexts to a temp directory at runtime.

---

## `internal/version`

Import path: `github.com/MartyFox/hive/internal/version`

#### Variables

```go
var Version   = "dev"
var Commit    = "unknown"
var BuildDate = "unknown"
```

Set at link time by the release workflow via `-ldflags -X`. Default values are used for local `go build` / `go run` invocations.

#### `String() string`

Returns the formatted version string: `"<version> (commit <commit>, built <date>)"`.

Example (release build): `"v1.2.3 (commit a1b2c3d, built 2025-06-01T12:00:00Z)"`
Example (local build): `"dev (commit unknown, built unknown)"`

---

## `cmd` (internal commands)

The `cmd` package is not importable, but the following are the primary entry points and shared helpers relevant to contributors.

#### `cmd.Execute()`

Root entry point called from `main.go`. Delegates to Cobra's `rootCmd.Execute()` and exits with code 1 on error.

#### `ensureImage(agent string) (string, error)`

Resolves the Podman image to use for an agent run via the three-step fallback:

1. Local image `hive-<agent>` exists → return it
2. Pull `<registry>/hive-<agent>:latest` → tag as local → return it
3. Pull failed → extract embedded Containerfiles → `buildAgent()` → return local

Uses seam variables (`imageExistsFunc`, `pullImageFunc`, etc.) for testability.

#### `buildTarget(target, ctxDir string, noCache bool) error`

Dispatches to `buildAll`, `buildBase`, or `buildSingleAgent` based on `target`. Shared by both `hive build` (`noCache=false`) and `hive update` (`noCache=true`).

#### `extractBuildContext() (dir string, cleanup func(), err error)`

Unpacks the embedded `imgfs.FS` to an OS temp directory. Returns the directory path and a cleanup function that removes it. Used by both `hive build`/`update` and as a fallback in `ensureImage`.

#### `shellQuote(s string) string`

Wraps `s` in single quotes with embedded single-quote escaping (`'` → `'\''`). Used only for `--cmd` shell composition — not for `--prompt` (which is passed as argv).
