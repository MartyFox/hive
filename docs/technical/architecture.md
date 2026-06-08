# Hive — Architecture

## Overview

Hive is a single statically linked Go binary. It embeds all Containerfile build contexts via `//go:embed` and delegates all container operations to the `podman` CLI subprocess. There is no daemon, no Docker socket, and no shared state beyond the user's `~/.hive/` directory.

```
┌─────────────────────────────────────────────────────────────────┐
│  hive binary                                                    │
│                                                                 │
│  cmd/            ←  Cobra CLI layer (user-facing commands)      │
│    root.go       ←  Execute() entry, command registration       │
│    run.go        ←  hive run: image resolution + podman exec    │
│    build.go      ←  hive build/update: context extraction       │
│    update.go     ←  hive update: delegates to build.go          │
│    list.go       ←  hive list: podman images passthrough        │
│    version.go    ←  hive version: version string output         │
│                                                                 │
│  internal/podman/  ←  All Podman integration and config logic   │
│    podman.go       ←  Config, mounts, run args, token handling  │
│                                                                 │
│  internal/imgfs/   ←  Embedded Containerfile build contexts     │
│    imgfs.go        ←  Exports embed.FS via //go:embed           │
│    images/         ←  base/ claude/ copilot/ gemini/ codex/     │
│                                                                 │
│  internal/version/ ←  Build metadata (ldflags injection)        │
│    version.go      ←  Version, Commit, BuildDate variables      │
└─────────────────────────────────────────────────────────────────┘
```

## Package Responsibilities

### `cmd/`

Cobra command layer. Owns CLI argument parsing, flag definitions, and top-level orchestration. Commands call into `internal/podman` for all business logic. Commands do not call each other except that `build.go` contains shared helpers (`buildTarget`, `buildAll`, `buildAgent`, `buildBase`) used by both `hive build` and `hive update`.

`run.go` uses seam variables (`imageExistsFunc`, `pullImageFunc`, etc.) to allow test injection without a running Podman daemon.

### `internal/podman`

All Podman integration and host configuration logic. Single file: `podman.go`. Responsibilities:

- Config resolution (env → `~/.hive/config` → `~/.hive/config.yaml` → default)
- Mount argument construction for agent config, Hive state, and extra mounts
- Path validation and sensitive-path rejection
- GitHub token injection (Podman secret or env-file modes)
- Corporate CA cert injection at runtime
- `podman run` argument assembly via `BuildRunArgs`
- Image existence, pull, tag, build, and network operations

No package in `cmd/` contains config or mount logic — all of it lives here.

### `internal/imgfs`

Exposes a single `embed.FS` variable (`FS`) containing the full `images/` tree. Consumed by `cmd/build.go`'s `extractBuildContextFromFS` to unpack build contexts to a temp directory at runtime.

### `internal/version`

Three `var` declarations (`Version`, `Commit`, `BuildDate`) with `dev`/`unknown` defaults for local builds. The release workflow overwrites these at link time via `-ldflags -X`.

## Startup Flow

### `hive run <agent>`

```
main() → cmd.Execute()
  → runRun()
      1. ValidAgent(agent)          — reject unknown agents early, before any I/O
      2. setupAgentRun(agent)
           CheckPodman()            — verify podman binary; start machine on macOS
           EnsureNetwork()          — create hive-net bridge if absent
           CleanCopilotMCPConfig()  — remove stale hive-generated mcp-config.json (copilot only)
      3. ensureImage(agent)
           ImageExists(local)       → use local image
           PullImage(registry)      → tag as local → use it
           pull failed              → extractBuildContext() → buildAgent() → use local
      4. runOptions()               — build RunOptions from flags + config
      5. Branch on mode:
           --prompt set             → executePromptRun()  → runPodmanChild()
           --cmd set                → podman.BuildRunArgs() → executeCommandRun() → runPodmanChild()
           token injection active   → podman.BuildRunArgs() → runPodmanChild()
           interactive, no token   → podman.BuildRunArgs() → execPodman() [syscall.Exec]
```

`syscall.Exec` replaces the hive process with Podman for interactive sessions with no token cleanup needed. Prompt, command, and token-cleanup paths use `exec.Command` (child process). Child Podman runs receive a temporary `--cidfile`; on SIGINT/SIGTERM Hive stops the recorded container before exiting so `--prompt` runs do not leave token-consuming containers behind.

### `hive build [target]`

```
runBuild()
  → CheckPodman()
  → extractBuildContext()      — unpack embed.FS to OS temp dir
  → buildTarget(target, ctxDir, noCache=false)
       "all"    → buildAll()   → buildBase() then buildAgent() for each agent
       "base"   → buildBase()
       default  → buildSingleAgent() → buildAgent() (auto-builds base if absent)
  → cleanup()                  — remove temp dir
```

`hive update` is identical with `noCache=true`.

## Config Resolution

Config keys are resolved in this precedence order (highest first):

```
Environment variable
  └─ ~/.hive/config  (KEY=VALUE, legacy)
       └─ ~/.hive/config.yaml  (structured YAML)
            └─ hardcoded default
```

`hiveConfigVal(key)` handles env → legacy file. `hiveConfigValDefault(key, yamlValue, fallback)` adds YAML and default layers. Both live in `internal/podman/podman.go`.

## Mount Construction

`BuildRunArgs(agent, opts)` assembles all `podman run` arguments in this order:

1. `baseRunArgs` — `--rm`, `-it`, `--cap-drop=ALL`, `--security-opt no-new-privileges`, `--network`, `-v $PWD:/workspace:rw,z`, `--workdir`
2. `appendConfigMountArgs` — agent config dir + `~/.agents`, both `ro` by default; `rw` when `opts.WritableConfig` or `HIVE_AGENT_CONFIG_MODE=read-write`/`rw`
3. `appendStateMountArgs` — `~/.hive/state/<agent>/` → `/home/agent/.hive-state:rw,z` (created if absent)
4. `appendAgentStateEnvArgs` — Copilot read-only mode sets `COPILOT_HOME=/home/agent/.hive-state/copilot-home`
5. `appendExtraMountArgs` — YAML `mounts[]` entries after validation
6. `appendTokenArgs` — Podman secret or env-file for GitHub token; no-op when off
7. `appendCertArgs` — `~/.hive/extra-ca.pem` bind mount + `NODE_EXTRA_CA_CERTS` env (no-op when absent)

## Path Validation

Extra YAML mounts and `_HOME` config overrides go through layered validation:

```
validateExtraHostPath(path, allowDangerous)
  → validateHostPath(path, allowDangerous)
      expandHome()            — expand ~ prefix
      reject $VAR             — no shell variable expansion
      require abs path
      if !allowDangerous: reject / and $HOME
  → if !allowDangerous: isSensitiveParent()
      checks against: ~/.ssh  ~/.gnupg  ~/.aws  ~/.config/gcloud  ~/.kube
      plus all five default agent config paths
```

`_HOME` override keys (`CLAUDE_HOME`, etc.) go through `validateConfigHostPath`, which additionally allows the five standard agent config paths but rejects anything broader.

## Token Injection Lifecycle

```
effectiveGitHubTokenMode(flagOptIn)
  configuredGitHubTokenMode()  — env/config/yaml/default ("off")
  normalizeGitHubTokenMode()   — canonicalise aliases

mode = "off"          → log "off", no args added, cleanup = no-op
mode = "env-file"     → writeTokenEnvFile() → --env-file <tmp>
                         cleanup: sync.Once os.Remove(tmp)
mode = "podman-secret" → createPodmanSecret(tok) per env var
                          copilot gets two secrets (GH_TOKEN + GITHUB_PERSONAL_ACCESS_TOKEN)
                          cleanup: sync.Once removePodmanSecrets(names)
```

Cleanup runs via `defer cleanup()` in `runPodmanChild`. For `syscall.Exec` paths (no token), there is nothing to clean up.

## Test Seams

`cmd/run.go` declares package-level function variables for all external calls made by `ensureImage`:

```go
var (
    imageExistsFunc         = podman.ImageExists
    registryNameFunc        = podman.RegistryName
    pullImageFunc           = podman.PullImage
    tagImageFunc            = podman.TagImage
    extractBuildContextFunc = extractBuildContext
    buildAgentForRunFunc    = buildAgent
)
```

Tests in `run_test.go` replace these to control behaviour without a running Podman daemon. Reset via `t.Cleanup` in `resetEnsureImageSeams`.
