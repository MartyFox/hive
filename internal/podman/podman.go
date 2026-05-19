package podman

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const (
	defaultNetwork  = "hive-net"
	Prefix          = "hive"
	defaultRegistry = "ghcr.io/martyfox"
	defaultBeadsVer = "1.0.4"
)

// Network returns the effective Podman network name.
// Override with HIVE_NETWORK in ~/.hive/config or env.
func Network() string { return hiveConfigValDefault("HIVE_NETWORK", defaultNetwork) }

// registry returns the effective image registry base URL.
// Override with HIVE_REGISTRY in ~/.hive/config or env.
func registry() string { return hiveConfigValDefault("HIVE_REGISTRY", defaultRegistry) }

// CopilotHome returns the effective copilot config directory path.
// Override with COPILOT_HOME in ~/.hive/config or env.
func CopilotHome() string {
	home, _ := os.UserHomeDir()
	return hiveConfigValDefault("COPILOT_HOME", home+"/.copilot")
}

// AgentsHome returns the host path for the shared ~/.agents directory (skills, agents).
// Override with AGENTS_HOME in ~/.hive/config or env.
func AgentsHome() string {
	home, _ := os.UserHomeDir()
	return hiveConfigValDefault("AGENTS_HOME", home+"/.agents")
}

var agents = []string{"claude", "copilot", "gemini", "codex"}

// Agents returns the list of supported agent names.
func Agents() []string { return append([]string(nil), agents...) }

// ValidAgent reports whether name is a supported agent.
func ValidAgent(name string) bool {
	for _, a := range agents {
		if a == name {
			return true
		}
	}
	return false
}

// RegistryName returns the full registry image reference for an agent.
func RegistryName(agent string) string {
	return registry() + "/" + Prefix + "-" + agent + ":latest"
}

// CheckPodman verifies that podman is available and, on macOS, that the
// Podman Machine is running (starting it automatically if not).
func CheckPodman() error {
	if _, err := exec.LookPath("podman"); err != nil {
		return fmt.Errorf("podman not found — install Podman Desktop: https://podman-desktop.io")
	}
	if runtime.GOOS == "darwin" {
		probe := exec.Command("podman", "info")
		probe.Stdout = nil
		probe.Stderr = nil
		if probe.Run() != nil {
			fmt.Fprintln(os.Stderr, "[hive] Podman machine not running — starting...")
			start := exec.Command("podman", "machine", "start")
			start.Stdout = os.Stdout
			start.Stderr = os.Stderr
			if err := start.Run(); err != nil {
				return fmt.Errorf("failed to start Podman machine: %w\nRun: podman machine init", err)
			}
		}
	}
	return nil
}

// ImageExists reports whether a local image with the given name exists.
func ImageExists(name string) bool {
	return exec.Command("podman", "image", "exists", name).Run() == nil
}

// tlsVerifyFlag returns "--tls-verify=false" when HIVE_TLS_VERIFY=false is set
// via env or ~/.hive/config (useful behind TLS-intercepting proxies).
// TLS verification is enabled by default; set HIVE_TLS_VERIFY=false to opt out.
func tlsVerifyFlag() string {
	if strings.ToLower(hiveConfigVal("HIVE_TLS_VERIFY")) == "false" {
		fmt.Fprintln(os.Stderr, "[hive warn] TLS verification disabled — HIVE_TLS_VERIFY=false is set")
		return "--tls-verify=false"
	}
	return ""
}

// PullImage pulls an image from a registry, streaming output to stdout/stderr.
func PullImage(name string) error {
	args := []string{"pull"}
	if f := tlsVerifyFlag(); f != "" {
		args = append(args, f)
	}
	args = append(args, name)
	cmd := exec.Command("podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// TagImage tags src as dst.
func TagImage(src, dst string) error {
	cmd := exec.Command("podman", "tag", src, dst)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// EnsureNetwork creates the hive bridge network if it does not already exist.
func EnsureNetwork() error {
	probe := exec.Command("podman", "network", "inspect", Network())
	probe.Stdout = nil
	probe.Stderr = nil
	if probe.Run() == nil {
		return nil
	}
	create := exec.Command("podman", "network", "create",
		"--driver", "bridge",
		"--label", "hive.managed=true",
		Network(),
	)
	create.Stdout = os.Stdout
	create.Stderr = os.Stderr
	return create.Run()
}

// hiveConfig reads ~/.hive/config (KEY=VALUE lines) and returns the map.
// Missing file returns empty map. Malformed lines are silently skipped.
func hiveConfig() map[string]string {
	home, _ := os.UserHomeDir()
	data, err := os.ReadFile(home + "/.hive/config")
	if err != nil {
		return map[string]string{}
	}
	cfg := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			cfg[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return cfg
}

// hiveConfigVal returns the value for key, preferring the environment variable
// over ~/.hive/config (env always wins).
func hiveConfigVal(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return hiveConfig()[key]
}

// hiveConfigValDefault returns env var, then ~/.hive/config value, then fallback.
func hiveConfigValDefault(key, fallback string) string {
	if v := hiveConfigVal(key); v != "" {
		return v
	}
	return fallback
}

// ConfigValDefault exposes hive config resolution (env -> ~/.hive/config -> fallback)
// for command-layer feature flags.
func ConfigValDefault(key, fallback string) string {
	return hiveConfigValDefault(key, fallback)
}

// extraCACertPath returns the host path of ~/.hive/extra-ca.pem if it exists,
// or empty string (no-op path).
func extraCACertPath() string {
	home, _ := os.UserHomeDir()
	p := home + "/.hive/extra-ca.pem"
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// InjectCertToContext writes ~/.hive/extra-ca.pem into contextDir as extra-ca.pem
// when present. If no cert exists, this is a no-op.
func InjectCertToContext(contextDir string) error {
	dst := filepath.Join(contextDir, "extra-ca.pem")
	if certPath := extraCACertPath(); certPath != "" {
		data, err := os.ReadFile(certPath)
		if err != nil {
			return fmt.Errorf("reading %s: %w", certPath, err)
		}
		fmt.Println("[hive] Injecting ~/.hive/extra-ca.pem into build context")
		return os.WriteFile(dst, data, 0o644)
	}
	return nil
}

// BuildImage runs podman build -t tag contextDir with optional build args.
func BuildImage(tag, contextDir string, noCache bool, buildArgs []string) error {
	args := []string{"build"}
	if f := tlsVerifyFlag(); f != "" {
		args = append(args, f)
	}
	args = append(args, "-t", tag)
	if noCache {
		args = append(args, "--no-cache")
	}
	for _, ba := range buildArgs {
		args = append(args, "--build-arg", ba)
	}
	args = append(args, contextDir)
	cmd := exec.Command("podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// configMount is a single host→container bind mount for agent config.
type configMount struct {
	src  string
	dst  string
	desc string // shown in startup log
}

// configMounts returns all config bind mounts for an agent.
// Agent home paths default to standard locations but can be overridden
// via CLAUDE_HOME, COPILOT_HOME, GEMINI_HOME, CODEX_HOME, AGENTS_HOME in ~/.hive/config or env.
func configMounts(agent string) []configMount {
	home, _ := os.UserHomeDir()
	var mounts []configMount

	switch agent {
	case "claude":
		src := hiveConfigValDefault("CLAUDE_HOME", home+"/.claude")
		mounts = append(mounts, configMount{src, "/home/agent/.claude", "claude config"})
	case "copilot":
		src := hiveConfigValDefault("COPILOT_HOME", home+"/.copilot")
		mounts = append(mounts, configMount{src, "/home/agent/.copilot", "copilot config"})
	case "gemini":
		src := hiveConfigValDefault("GEMINI_HOME", home+"/.gemini")
		mounts = append(mounts, configMount{src, "/home/agent/.gemini", "gemini config"})
	case "codex":
		src := hiveConfigValDefault("CODEX_HOME", home+"/.config/openai")
		mounts = append(mounts, configMount{src, "/home/agent/.config/openai", "codex config"})
	}

	// ~/.agents/ is the shared personal skills/agents directory read by copilot CLI,
	// claude code, and other agents. Mount for all agents.
	mounts = append(mounts, configMount{AgentsHome(), "/home/agent/.agents", "shared agent skills"})

	return mounts
}

// ghToken tries to obtain the current gh CLI token from the host.
// On macOS, gh stores tokens in the Keychain rather than ~/.config/gh/,
// so we call "gh auth token" and inject the result as GH_TOKEN into the container.
// gh inside the container honours GH_TOKEN, so github-mcp-server's "gh auth token"
// call will succeed without needing any file mount.
//
// Trust note: this calls the first "gh" found on $PATH. On a developer workstation
// this is acceptable; in automated/CI contexts ensure $PATH is locked down.
func ghToken() string {
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// BuildRunArgs builds the podman run argument slice (after "podman", before image name).
// interactive should be true for REPL sessions (adds -it).
//
// Tokens (GH_TOKEN / GITHUB_PERSONAL_ACCESS_TOKEN) are written to a 0600 temp
// file and passed via --env-file so they do not appear in the process argument
// list (ps aux). The returned cleanup func removes that file; call it after the
// container exits. For syscall.Exec paths the cleanup cannot run — the temp
// file is left in /tmp (0600, cleaned by the OS).
func BuildRunArgs(agent string, interactive bool) ([]string, func()) {
	wd, _ := os.Getwd()
	cleanup := func() {} // no-op unless a secrets file is written

	args := []string{"run", "--rm"}
	if interactive {
		args = append(args, "-it")
	}
	args = append(args,
		"--cap-drop=ALL",
		"--security-opt", "no-new-privileges",
		"--network", Network(),
		"-v", wd+":/workspace:rw,z",
		"--workdir", "/workspace",
	)

	for _, m := range configMounts(agent) {
		if _, err := os.Stat(m.src); err == nil {
			args = append(args, "-v", m.src+":"+m.dst+":rw,z")
			fmt.Printf("[hive] %-36s → %s\n", m.desc, m.dst)
		} else {
			fmt.Fprintf(os.Stderr,
				"[hive warn] %s not found: %s\n  create with: mkdir -p %s\n",
				m.desc, m.src, m.src)
		}
	}

	// Inject gh auth token for all agents: gh CLI is installed in the base
	// image so every agent can use it (clone, PR, etc.).
	// GH_TOKEN      — read by gh CLI (all agents)
	// GITHUB_PERSONAL_ACCESS_TOKEN — read by github-mcp-server (copilot only)
	//
	// Tokens are written to a 0600 temp file and passed via --env-file to
	// keep them out of the process argument list (ps aux).
	if tok := ghToken(); tok != "" {
		vars := "GH_TOKEN=" + tok + "\n"
		if agent == "copilot" {
			vars += "GITHUB_PERSONAL_ACCESS_TOKEN=" + tok + "\n"
		}
		if f, err := os.CreateTemp("", "hive-secrets-*"); err == nil {
			_ = f.Chmod(0o600)
			_, _ = f.WriteString(vars)
			_ = f.Close()
			name := f.Name()
			args = append(args, "--env-file", name)
			var once sync.Once
			cleanup = func() { once.Do(func() { os.Remove(name) }) }
			fmt.Println("[hive] GH_TOKEN injected (all agents)")
			if agent == "copilot" {
				fmt.Println("[hive] GITHUB_PERSONAL_ACCESS_TOKEN injected for MCP server")
			}
		}
	} else {
		fmt.Fprintln(os.Stderr,
			"[hive warn] gh auth token not found — gh CLI inside container will be unauthenticated\n"+
				"  Fix: run 'gh auth login' on the host, then retry")
	}

	// Node.js ignores the system CA store and uses its own bundled CAs.
	// Bind-mount ~/.hive/extra-ca.pem directly into the container and point
	// NODE_EXTRA_CA_CERTS at it. Using a bind-mount means:
	//   1. No rebuild needed after cert rotation — always uses current host cert.
	//   2. Works even if the image was built without EXTRA_CA_CERT.
	//   3. NODE_EXTRA_CA_CERTS never points at an empty file.
	if certPath := extraCACertPath(); certPath != "" {
		args = append(args, "-v", certPath+":/run/certs/extra-ca.pem:ro,z")
		args = append(args, "-e", "NODE_EXTRA_CA_CERTS=/run/certs/extra-ca.pem")
		fmt.Println("[hive] NODE_EXTRA_CA_CERTS → ~/.hive/extra-ca.pem (proxy)")
	}

	// Pass through OpenTelemetry settings so in-container CLIs can emit
	// trace/metric data and use Copilot-specific telemetry options.
	for _, k := range []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_HEADERS",
		"OTEL_EXPORTER_OTLP_PROTOCOL",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
		"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
		"OTEL_RESOURCE_ATTRIBUTES",
		"OTEL_SERVICE_NAME",
		"OTEL_LOG_LEVEL",
		"OTEL_TRACES_EXPORTER",
		"OTEL_METRICS_EXPORTER",
		"OTEL_LOGS_EXPORTER",
		"COPILOT_OTEL_ENABLED",
		"COPILOT_OTEL_EXPORTER_TYPE",
		"COPILOT_OTEL_FILE_EXPORTER_PATH",
		"COPILOT_OTEL_SOURCE_NAME",
	} {
		if v := hiveConfigVal(k); v != "" {
			args = append(args, "-e", k+"="+v)
		}
	}

	return args, cleanup
}

// BeadsEnabled reports whether automatic bd init should run before --cmd tasks.
// Opt-in: set HIVE_BEADS=1 in ~/.hive/config or the environment.
func BeadsEnabled() bool {
	return hiveConfigVal("HIVE_BEADS") == "1"
}

// BeadsArg returns the --build-arg value for HIVE_BEADS to pass during image
// builds, reflecting the current config setting.
func BeadsArg() string {
	if BeadsEnabled() {
		return "HIVE_BEADS=1"
	}
	return "HIVE_BEADS=0"
}

// BeadsVersionArg returns the --build-arg value for HIVE_BEADS_VERSION to pass
// during image builds, reflecting config with a safe default.
func BeadsVersionArg() string {
	return "HIVE_BEADS_VERSION=" + hiveConfigValDefault("HIVE_BEADS_VERSION", defaultBeadsVer)
}

// CleanCopilotMCPConfig removes any hive-generated mcp-config.json from the
// copilot home directory. Copilot CLI v1.0.44+ uses a remote SSE transport for
// the built-in GitHub MCP server — no local binary or custom config needed.
// Leaving a hive-generated file causes a name collision (both register as
// "github-mcp-server") and the built-in connection fails.
//
// Only removes files that contain the hive fingerprint. User-created files
// (with entries other than github-mcp-server) are preserved.
func CleanCopilotMCPConfig() {
	copilotHome := CopilotHome()
	mcpPath := filepath.Join(copilotHome, "mcp-config.json")

	data, err := os.ReadFile(mcpPath)
	if err != nil {
		return // file absent — nothing to do
	}
	content := string(data)

	// Hive fingerprint: only our generated entry present.
	// If user has additional entries we leave the file alone.
	if !strings.Contains(content, `"github-mcp-server"`) {
		return // not ours
	}
	if strings.Contains(content, `"command": "/usr/local/bin/github-mcp-server"`) &&
		!hasOtherMCPEntries(content) {
		if err := os.Remove(mcpPath); err != nil {
			fmt.Fprintf(os.Stderr, "[hive] MCP: could not remove %s: %v\n", mcpPath, err)
			return
		}
		fmt.Println("[hive] MCP: removed hive-generated mcp-config.json (built-in remote transport handles this)")
	}
}

// hasOtherMCPEntries reports whether content contains MCP server entries
// beyond the hive-generated github-mcp-server entry.
func hasOtherMCPEntries(content string) bool {
	var cfg struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(content), &cfg); err != nil {
		// Unparseable JSON — be conservative and don't delete.
		return true
	}
	for k := range cfg.MCPServers {
		if k != "github-mcp-server" {
			return true
		}
	}
	return false
}

// JoinAgents returns a space-joined string of all agent names.
func JoinAgents() string {
	return " " + strings.Join(agents, " ")
}
