# Hive — Adding a New Agent

This guide covers every code and file change required to add a new agent (e.g. `myagent`) to hive. The changes span four locations: the Containerfile, the agent registry, the config mount wiring, and the test suite.

---

## 1. Create the Containerfile

Create `internal/imgfs/images/myagent/Containerfile`.

All agent images build from `hive-base`. The minimal structure is:

```dockerfile
# hive-myagent — <Agent name> CLI in a Podman sandbox
FROM hive-base

LABEL org.opencontainers.image.source="https://github.com/MartyFox/hive" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.description="Hive <Agent name> agent image"

# Pin to a specific version. Do not use 'latest' — versions are updated
# explicitly via hive update to avoid silent breaking changes.
RUN npm install -g @myorg/myagent-cli@1.2.3

USER agent

ENTRYPOINT ["myagent", "--auto-approve"]
```

### Requirements

- **Base**: always `FROM hive-base`. Never use a public base directly — the base image provides the non-root `agent` user, system tools, CA handling, and optional Beads.
- **User**: switch to `USER agent` before `ENTRYPOINT`. The `agent` user (uid 1000) is created in the base image.
- **Entrypoint**: use the agent binary's high-autonomy flag if one exists. This is how all existing agents operate (`--dangerously-skip-permissions`, `--yolo`).
- **Version pin**: pin the npm package version in the `RUN npm install -g` line. The `hive update` command rebuilds with `--no-cache` to pick up newer versions.
- **OCI labels**: include the three standard labels. Use `image.description` to identify the agent.

If the agent requires additional system packages or setup steps (e.g. creating a config directory), add them before the `USER agent` line.

**Copilot example** — creating a directory as the agent user:

```dockerfile
USER agent
RUN mkdir -p /home/agent/.config/gh
ENTRYPOINT ["copilot", "--yolo"]
```

---

## 2. Register the Agent

Edit `internal/podman/podman.go`. Add the agent name to the `agents` slice:

```go
// Before
var agents = []string{"claude", "copilot", "gemini", "codex"}

// After
var agents = []string{"claude", "copilot", "gemini", "codex", "myagent"}
```

This single change propagates the agent name through:

- `ValidAgent()` — CLI argument validation
- `Agents()` — build loops in `buildAll` and `runUpdate`
- `JoinAgents()` — error messages listing valid agents
- `RegistryName()` — GHCR pull reference construction

---

## 3. Wire the Config Mount

Edit `agentConfigMount()` in `internal/podman/podman.go`. Add a `case` for the new agent:

```go
func agentConfigMount(agent string) (configMount, bool) {
    home, _ := os.UserHomeDir()
    paths := yamlConfig().AgentConfig.Paths
    switch agent {
    // ... existing cases ...
    case "myagent":
        src := hiveConfigValDefault("MYAGENT_HOME", paths.MyAgent, home+"/.myagent")
        return configMount{src, "/home/agent/.myagent", "myagent config"}, true
    default:
        return configMount{}, false
    }
}
```

### Add the YAML config path field

Edit the `hiveYAMLConfig` struct's `AgentConfig.Paths` to add the new field:

```go
Paths struct {
    Claude  string `yaml:"claude"`
    Copilot string `yaml:"copilot"`
    Gemini  string `yaml:"gemini"`
    Codex   string `yaml:"codex"`
    Agents  string `yaml:"agents"`
    MyAgent string `yaml:"myagent"`   // add this
} `yaml:"paths"`
```

### Add the env override key to the sensitive-path allowlist

Edit `defaultAgentConfigPaths()` to include the new default path. This allows the default config path to be used as a `_HOME` override without triggering the sensitive-path rejection:

```go
func defaultAgentConfigPaths(home string) []string {
    return []string{
        filepath.Clean(filepath.Join(home, ".claude")),
        filepath.Clean(filepath.Join(home, ".copilot")),
        filepath.Clean(filepath.Join(home, ".gemini")),
        filepath.Clean(filepath.Join(home, ".config", "openai")),
        filepath.Clean(filepath.Join(home, ".agents")),
        filepath.Clean(filepath.Join(home, ".myagent")),  // add this
    }
}
```

---

## 4. Wire `--prompt` Support (if applicable)

If the agent CLI accepts a non-interactive prompt flag, add it to `promptEntrypointArgs()` in `cmd/run.go`:

```go
func promptEntrypointArgs(agent, prompt string) (string, []string, bool) {
    switch agent {
    case "copilot":
        return "", []string{"--prompt", prompt}, true
    case "claude":
        return "", []string{"-p", prompt}, true
    case "myagent":
        return "", []string{"--prompt", prompt}, true
    default:
        return "", nil, false
    }
}
```

Return an empty entrypoint when the image default entrypoint already contains the right wrapper or approval flags. Use a non-empty entrypoint only when prompt mode must bypass the default image entrypoint.

If the agent does not support prompt mode, leave the `default` case — `--prompt myagent` will return a clear error to the user.

---

## 5. Update the CI Workflow

Edit `.github/workflows/build-images.yml`. The agent list in the `Build and push agent images` step is hardcoded:

```yaml
for agent in claude copilot gemini codex; do
```

Add the new agent:

```yaml
for agent in claude copilot gemini codex myagent; do
```

---

## 6. Update Tests

### `cmd/workflow_test.go` — CI agent list parity

This test verifies that the workflow agent list matches the embedded image directories. It will fail if the workflow is updated but the Containerfile is missing, or vice versa. Run it to confirm your changes are consistent:

```bash
go test ./cmd/ -run TestWorkflowAgentListMatchesEmbeddedImages
```

### `internal/podman/podman_test.go` — config mount

Add a test case to the `agentConfigMount` test for the new agent:

```go
{"myagent", home + "/.myagent", "/home/agent/.myagent"},
```

### New agent-specific tests

Consider adding:

- A test verifying `ValidAgent("myagent")` returns true
- A test verifying `agentConfigMount("myagent")` returns the expected paths
- A test verifying `MYAGENT_HOME` override is respected

---

## 7. Update Documentation

- **`README.md`** — Add `myagent` to the Quick Start examples, the Agents and Approval Mode table, and the Global Config table.
- **`internal/imgfs/images/myagent/Containerfile`** — Ensure the comment header and OCI labels are accurate.
- **`MAP.md`** — Update the Package Index, CLI Command Surface, and Configuration Schema sections.

---

## Checklist

- [ ] `internal/imgfs/images/myagent/Containerfile` created
- [ ] `agents` slice in `podman.go` updated
- [ ] `agentConfigMount()` case added in `podman.go`
- [ ] `hiveYAMLConfig.AgentConfig.Paths` field added
- [ ] `defaultAgentConfigPaths()` updated
- [ ] `promptEntrypointArgs()` updated (or confirmed not needed)
- [ ] `.github/workflows/build-images.yml` agent list updated
- [ ] `go test ./...` passes
- [ ] README and MAP.md updated
