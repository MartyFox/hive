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
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultNetwork  = "hive-net"
	Prefix          = "hive"
	defaultRegistry = "ghcr.io/martyfox"
	defaultBeadsVer = "1.0.4"
)

// Network returns the effective Podman network name.
// Override with HIVE_NETWORK in env or ~/.hive/config, or network in ~/.hive/config.yaml.
func Network() string {
	return hiveConfigValDefault("HIVE_NETWORK", yamlConfig().Network, defaultNetwork)
}

// registry returns the effective image registry base URL.
// Override with HIVE_REGISTRY in env or ~/.hive/config, or registry in ~/.hive/config.yaml.
func registry() string {
	return hiveConfigValDefault("HIVE_REGISTRY", yamlConfig().Registry, defaultRegistry)
}

// CopilotHome returns the effective copilot config directory path.
// Override with COPILOT_HOME in env or ~/.hive/config, or agentConfig.paths.copilot in ~/.hive/config.yaml.
func CopilotHome() string {
	home, _ := os.UserHomeDir()
	return hiveConfigValDefault("COPILOT_HOME", yamlConfig().AgentConfig.Paths.Copilot, home+"/.copilot")
}

// AgentsHome returns the host path for the shared ~/.agents directory (skills, agents).
// Override with AGENTS_HOME in env or ~/.hive/config, or agentConfig.paths.agents in ~/.hive/config.yaml.
func AgentsHome() string {
	home, _ := os.UserHomeDir()
	return hiveConfigValDefault("AGENTS_HOME", yamlConfig().AgentConfig.Paths.Agents, home+"/.agents")
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

// tlsVerifyFlag returns "--tls-verify=false" when TLS verification is disabled
// via env, ~/.hive/config, or ~/.hive/config.yaml.
// TLS verification is enabled by default; set HIVE_TLS_VERIFY=false to opt out.
func tlsVerifyFlag() string {
	disabled := strings.ToLower(hiveConfigVal("HIVE_TLS_VERIFY")) == "false"
	if hiveConfigVal("HIVE_TLS_VERIFY") == "" && yamlConfig().TLSVerify != nil {
		disabled = !*yamlConfig().TLSVerify
	}
	if disabled {
		fmt.Fprintln(os.Stderr, "[hive warn] TLS verification disabled")
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

type hiveYAMLConfig struct {
	Network   string `yaml:"network"`
	Registry  string `yaml:"registry"`
	TLSVerify *bool  `yaml:"tlsVerify"`
	GitHub    struct {
		TokenMode string `yaml:"tokenMode"`
	} `yaml:"github"`
	AgentConfig struct {
		Mode  string `yaml:"mode"`
		Paths struct {
			Claude  string `yaml:"claude"`
			Copilot string `yaml:"copilot"`
			Gemini  string `yaml:"gemini"`
			Codex   string `yaml:"codex"`
			Agents  string `yaml:"agents"`
		} `yaml:"paths"`
	} `yaml:"agentConfig"`
	Mounts []yamlMount `yaml:"mounts"`
}

type yamlMount struct {
	Name                   string `yaml:"name"`
	Host                   string `yaml:"host"`
	Container              string `yaml:"container"`
	Mode                   string `yaml:"mode"`
	AllowDangerousHostPath bool   `yaml:"allowDangerousHostPath"`
}

// yamlConfig reads ~/.hive/config.yaml. Missing file returns zero values.
func yamlConfig() hiveYAMLConfig {
	home, _ := os.UserHomeDir()
	data, err := os.ReadFile(filepath.Join(home, ".hive", "config.yaml"))
	if err != nil {
		return hiveYAMLConfig{}
	}
	var cfg hiveYAMLConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "[hive warn] ignoring ~/.hive/config.yaml: %v\n", err)
		return hiveYAMLConfig{}
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

// hiveConfigValDefault returns env var, then ~/.hive/config, then YAML value, then fallback.
func hiveConfigValDefault(key, yamlValue, fallback string) string {
	if v := hiveConfigVal(key); v != "" {
		return v
	}
	if yamlValue != "" {
		return yamlValue
	}
	return fallback
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

type runtimeMount struct {
	src  string
	dst  string
	mode string
	desc string
}

// RunOptions controls host integration for a podman run.
type RunOptions struct {
	Interactive    bool
	WritableConfig bool
	GitHubToken    bool
}

// AgentConfigWritable reports whether host agent config should be mounted rw.
func AgentConfigWritable() bool {
	mode := normalizeAgentConfigMode(hiveConfigValDefault("HIVE_AGENT_CONFIG_MODE", yamlConfig().AgentConfig.Mode, "read-only"))
	return mode == "rw"
}

func normalizeAgentConfigMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "read-only", "ro":
		return "ro"
	case "read-write", "rw", "writable":
		return "rw"
	default:
		return "ro"
	}
}

// GitHubTokenEnabled reports whether host gh auth should be injected.
func GitHubTokenEnabled() bool {
	return normalizeGitHubTokenMode(configuredGitHubTokenMode()) != "off"
}

// configMounts returns all config bind mounts for an agent.
// Agent home paths default to standard locations but can be overridden
// via env, ~/.hive/config, or ~/.hive/config.yaml.
func configMounts(agent string) ([]configMount, error) {
	var mounts []configMount
	if m, ok := agentConfigMount(agent); ok {
		mounts = append(mounts, m)
	}
	// ~/.agents/ is the shared personal skills/agents directory read by copilot CLI,
	// claude code, and other agents. Mount for all agents.
	mounts = append(mounts, configMount{AgentsHome(), "/home/agent/.agents", "shared agent skills"})
	return validateConfigMounts(mounts)
}

func validateConfigMounts(mounts []configMount) ([]configMount, error) {
	for i := range mounts {
		clean, err := validateConfigHostPath(mounts[i].src)
		if err != nil {
			return nil, fmt.Errorf("%s host path: %w", mounts[i].desc, err)
		}
		mounts[i].src = clean
	}
	return mounts, nil
}

func agentConfigMount(agent string) (configMount, bool) {
	home, _ := os.UserHomeDir()
	paths := yamlConfig().AgentConfig.Paths
	switch agent {
	case "claude":
		src := hiveConfigValDefault("CLAUDE_HOME", paths.Claude, home+"/.claude")
		return configMount{src, "/home/agent/.claude", "claude config"}, true
	case "copilot":
		src := hiveConfigValDefault("COPILOT_HOME", paths.Copilot, home+"/.copilot")
		return configMount{src, "/home/agent/.copilot", "copilot config"}, true
	case "gemini":
		src := hiveConfigValDefault("GEMINI_HOME", paths.Gemini, home+"/.gemini")
		return configMount{src, "/home/agent/.gemini", "gemini config"}, true
	case "codex":
		src := hiveConfigValDefault("CODEX_HOME", paths.Codex, home+"/.config/openai")
		return configMount{src, "/home/agent/.config/openai", "codex config"}, true
	default:
		return configMount{}, false
	}
}

func extraMounts() ([]runtimeMount, error) {
	var mounts []runtimeMount
	for _, m := range yamlConfig().Mounts {
		host, container, mode, err := validateExtraMount(m)
		if err != nil {
			return nil, err
		}
		mounts = append(mounts, runtimeMount{
			src:  host,
			dst:  container,
			mode: mode,
			desc: m.Name,
		})
	}
	return mounts, nil
}

func validateExtraMount(m yamlMount) (string, string, string, error) {
	if strings.TrimSpace(m.Name) == "" {
		return "", "", "", fmt.Errorf("extra mount missing name")
	}
	host, err := validateExtraHostPath(m.Host, m.AllowDangerousHostPath)
	if err != nil {
		return "", "", "", fmt.Errorf("extra mount %q host path: %w", m.Name, err)
	}
	container, err := validateExtraContainerPath(m.Container)
	if err != nil {
		return "", "", "", fmt.Errorf("extra mount %q container path: %w", m.Name, err)
	}
	mode, err := normalizeMountMode(m.Mode)
	if err != nil {
		return "", "", "", fmt.Errorf("extra mount %q mode: %w", m.Name, err)
	}
	return host, container, mode, nil
}

func validateConfigHostPath(path string) (string, error) {
	clean, err := validateHostPath(path, false)
	if err != nil {
		return "", err
	}
	if isSensitiveConfigHostPath(clean) {
		return "", fmt.Errorf("%s exposes sensitive host config; use a more specific path", clean)
	}
	return clean, nil
}

func validateExtraHostPath(path string, allowDangerous bool) (string, error) {
	clean, err := validateHostPath(path, allowDangerous)
	if err != nil {
		return "", err
	}
	if !allowDangerous && isSensitiveParent(clean) {
		return "", fmt.Errorf("%s exposes sensitive host config; set allowDangerousHostPath: true to override", clean)
	}
	return clean, nil
}

func validateHostPath(path string, allowDangerous bool) (string, error) {
	expanded, err := expandHome(path)
	if err != nil {
		return "", err
	}
	if strings.Contains(expanded, "$") {
		return "", fmt.Errorf("shell variables are not expanded: %s", path)
	}
	if !filepath.IsAbs(expanded) {
		return "", fmt.Errorf("must be absolute or start with ~: %s", path)
	}
	clean := filepath.Clean(expanded)
	if allowDangerous {
		return clean, nil
	}
	home, _ := os.UserHomeDir()
	home = filepath.Clean(home)
	if clean == string(os.PathSeparator) || clean == home {
		return "", fmt.Errorf("%s is too broad", clean)
	}
	return clean, nil
}

func expandHome(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("missing path")
	}
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home, nil
	}
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return path, nil
}

func validateExtraContainerPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("missing path")
	}
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("must be absolute: %s", path)
	}
	if strings.HasPrefix(clean, "/mnt/") {
		return clean, nil
	}
	return "", fmt.Errorf("must be under /mnt: %s", clean)
}

func normalizeMountMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "read-only", "ro":
		return "ro", nil
	case "writable", "read-write", "rw":
		return "rw", nil
	default:
		return "", fmt.Errorf("must be read-only or read-write")
	}
}

func isSensitiveParent(path string) bool {
	home, _ := os.UserHomeDir()
	for _, target := range sensitiveHostPaths(home) {
		if path == target || isParent(path, target) {
			return true
		}
	}
	return false
}

func isSensitiveConfigHostPath(path string) bool {
	home, _ := os.UserHomeDir()
	for _, allowed := range defaultAgentConfigPaths(home) {
		if path == allowed {
			return false
		}
	}
	return isSensitiveParent(path)
}

func defaultAgentConfigPaths(home string) []string {
	return []string{
		filepath.Clean(filepath.Join(home, ".claude")),
		filepath.Clean(filepath.Join(home, ".copilot")),
		filepath.Clean(filepath.Join(home, ".gemini")),
		filepath.Clean(filepath.Join(home, ".config", "openai")),
		filepath.Clean(filepath.Join(home, ".agents")),
	}
}

func sensitiveHostPaths(home string) []string {
	return append(defaultAgentConfigPaths(home),
		filepath.Clean(filepath.Join(home, ".ssh")),
		filepath.Clean(filepath.Join(home, ".gnupg")),
		filepath.Clean(filepath.Join(home, ".aws")),
		filepath.Clean(filepath.Join(home, ".config", "gcloud")),
		filepath.Clean(filepath.Join(home, ".kube")),
	)
}

func isParent(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
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
//
// Tokens (GH_TOKEN / GITHUB_PERSONAL_ACCESS_TOKEN) are only injected when
// explicitly requested. They are written to a 0600 temp file and passed via
// --env-file so they do not appear in the process argument list (ps aux). The
// returned cleanup func removes that file; callers must run Podman as a child
// process when secrets are enabled so cleanup can run.
func BuildRunArgs(agent string, opts RunOptions) ([]string, func(), error) {
	cleanup := func() {} // no-op unless a secrets file is written
	args := baseRunArgs(opts)

	var err error
	if args, err = appendConfigMountArgs(args, agent, opts); err != nil {
		return nil, cleanup, err
	}
	if args, err = appendStateMountArgs(args, agent); err != nil {
		return nil, cleanup, err
	}
	args = appendAgentStateEnvArgs(args, agent, opts)
	if args, err = appendExtraMountArgs(args); err != nil {
		return nil, cleanup, err
	}
	args, cleanup, err = appendTokenArgs(args, agent, opts)
	if err != nil {
		return nil, cleanup, err
	}
	args = appendCertArgs(args)

	return args, cleanup, nil
}

func baseRunArgs(opts RunOptions) []string {
	wd, _ := os.Getwd()
	args := []string{"run", "--rm"}
	if opts.Interactive {
		args = append(args, "-it")
	}
	return append(args,
		"--cap-drop=ALL",
		"--security-opt", "no-new-privileges",
		"--network", Network(),
		"-v", wd+":/workspace:rw,z",
		"--workdir", "/workspace",
	)
}

func appendConfigMountArgs(args []string, agent string, opts RunOptions) ([]string, error) {
	mode := configMountMode(opts)
	configs, err := configMounts(agent)
	if err != nil {
		return nil, err
	}
	for _, m := range configs {
		if _, err := os.Stat(m.src); err == nil {
			args = append(args, "-v", m.src+":"+m.dst+":"+mode+",z")
			fmt.Printf("[hive] %-36s → %s (%s)\n", m.desc, m.dst, mode)
			continue
		}
		fmt.Fprintf(os.Stderr,
			"[hive warn] %s not found: %s\n  create with: mkdir -p %s\n",
			m.desc, m.src, m.src)
	}
	return args, nil
}

func configMountMode(opts RunOptions) string {
	if agentConfigIsWritable(opts) {
		fmt.Fprintln(os.Stderr, "[hive warn] agent config mounted read-write")
		return "rw"
	}
	return "ro"
}

func agentConfigIsWritable(opts RunOptions) bool {
	return opts.WritableConfig || AgentConfigWritable()
}

func appendStateMountArgs(args []string, agent string) ([]string, error) {
	home, _ := os.UserHomeDir()
	statePath := filepath.Join(home, ".hive", "state", agent)
	if err := os.MkdirAll(statePath, 0o750); err != nil {
		return nil, fmt.Errorf("creating hive state directory %s: %w", statePath, err)
	}
	args = append(args, "-v", statePath+":/home/agent/.hive-state:rw,z")
	fmt.Printf("[hive] %-36s → %s (rw)\n", "hive agent state", "/home/agent/.hive-state")
	return args, nil
}

func appendAgentStateEnvArgs(args []string, agent string, opts RunOptions) []string {
	if agent != "copilot" || agentConfigIsWritable(opts) {
		return args
	}
	fmt.Println("[hive] Copilot runtime state → /home/agent/.hive-state/copilot-home (rw)")
	return append(args, "-e", "COPILOT_HOME=/home/agent/.hive-state/copilot-home")
}

func appendExtraMountArgs(args []string) ([]string, error) {
	extras, err := extraMounts()
	if err != nil {
		return nil, err
	}
	for _, m := range extras {
		if _, err := os.Stat(m.src); err != nil {
			return nil, fmt.Errorf("extra mount %q not found: %s", m.desc, m.src)
		}
		args = append(args, "-v", m.src+":"+m.dst+":"+m.mode+",z")
		fmt.Printf("[hive] %-36s → %s (%s)\n", "extra mount: "+m.desc, m.dst, m.mode)
	}
	return args, nil
}

func appendTokenArgs(args []string, agent string, opts RunOptions) ([]string, func(), error) {
	cleanup := func() {}
	mode := effectiveGitHubTokenMode(opts.GitHubToken)
	if mode == "off" {
		fmt.Println("[hive] GitHub token injection off")
		return args, cleanup, nil
	}
	if mode != "env-file" && mode != "podman-secret" {
		return nil, cleanup, fmt.Errorf("unsupported GitHub token mode %q", mode)
	}
	tok := ghToken()
	if tok == "" {
		fmt.Fprintln(os.Stderr,
			"[hive warn] gh auth token not found — gh CLI inside container will be unauthenticated\n"+
				"  Fix: run 'gh auth login' on the host, then retry")
		return args, cleanup, nil
	}
	switch mode {
	case "env-file":
		var err error
		args, cleanup, err = appendTokenEnvFile(args, agent, tok)
		return args, cleanup, err
	case "podman-secret":
		var err error
		args, cleanup, err = appendTokenPodmanSecret(args, agent, tok)
		return args, cleanup, err
	}
	return nil, cleanup, fmt.Errorf("unsupported GitHub token mode %q", mode)
}

func configuredGitHubTokenMode() string {
	return hiveConfigValDefault("HIVE_GH_TOKEN_MODE", yamlConfig().GitHub.TokenMode, "off")
}

func effectiveGitHubTokenMode(flagOptIn bool) string {
	mode := normalizeGitHubTokenMode(configuredGitHubTokenMode())
	if flagOptIn && mode == "off" {
		return "podman-secret"
	}
	return mode
}

func normalizeGitHubTokenMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "off", "false", "0", "none":
		return "off"
	case "env-file", "env", "true", "1", "on":
		return "env-file"
	case "podman-secret", "secret":
		return "podman-secret"
	default:
		return strings.ToLower(strings.TrimSpace(mode))
	}
}

func appendTokenEnvFile(args []string, agent, tok string) ([]string, func(), error) {
	name, err := writeTokenEnvFile(agent, tok)
	if err != nil {
		return nil, func() {}, err
	}
	args = append(args, "--env-file", name)
	var once sync.Once
	cleanup := func() { once.Do(func() { os.Remove(name) }) }
	logTokenInjection("env-file", agent)
	return args, cleanup, nil
}

func appendTokenPodmanSecret(args []string, agent, tok string) ([]string, func(), error) {
	ghSecret, err := createPodmanSecret(tok)
	if err != nil {
		return nil, func() {}, fmt.Errorf("creating GH_TOKEN podman secret: %w", err)
	}
	secrets := []string{ghSecret}
	args = append(args, "--secret", ghSecret+",type=env,target=GH_TOKEN")
	if agent == "copilot" {
		var copilotSecret string
		if copilotSecret, err = createPodmanSecret(tok); err != nil {
			removePodmanSecrets(secrets)
			return nil, func() {}, fmt.Errorf("creating GITHUB_PERSONAL_ACCESS_TOKEN podman secret: %w", err)
		}
		secrets = append(secrets, copilotSecret)
		args = append(args, "--secret", copilotSecret+",type=env,target=GITHUB_PERSONAL_ACCESS_TOKEN")
	}
	var once sync.Once
	cleanup := func() { once.Do(func() { removePodmanSecrets(secrets) }) }
	logTokenInjection("podman-secret", agent)
	return args, cleanup, nil
}

func tokenEnvFileContent(agent, tok string) string {
	vars := "GH_TOKEN=" + tok + "\n"
	if agent == "copilot" {
		vars += "GITHUB_PERSONAL_ACCESS_TOKEN=" + tok + "\n"
	}
	return vars
}

func writeTokenEnvFile(agent, tok string) (string, error) {
	f, err := os.CreateTemp("", "hive-secrets-*")
	if err != nil {
		return "", fmt.Errorf("creating GitHub token env-file: %w", err)
	}
	if err := f.Chmod(0o600); err != nil {
		return "", closeAndRemove(f, "securing GitHub token env-file: %w", err)
	}
	if _, err := f.WriteString(tokenEnvFileContent(agent, tok)); err != nil {
		return "", closeAndRemove(f, "writing GitHub token env-file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("closing GitHub token env-file: %w", err)
	}
	return f.Name(), nil
}

func closeAndRemove(f *os.File, format string, err error) error {
	_ = f.Close()
	_ = os.Remove(f.Name())
	return fmt.Errorf(format, err)
}

func createPodmanSecret(value string) (string, error) {
	name := fmt.Sprintf("hive-gh-token-%d-%d", os.Getpid(), time.Now().UnixNano())
	cmd := exec.Command("podman", "secret", "create", name, "-")
	cmd.Stdin = strings.NewReader(value)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return name, nil
}

func removePodmanSecrets(names []string) {
	for _, name := range names {
		removePodmanSecret(name)
	}
}

func removePodmanSecret(name string) {
	cmd := exec.Command("podman", "secret", "rm", name)
	cmd.Stdout = nil
	cmd.Stderr = nil
	_ = cmd.Run()
}

func logTokenInjection(mode, agent string) {
	fmt.Println("[hive] GitHub token injection " + mode)
	fmt.Println("[hive] GH_TOKEN injected")
	if agent == "copilot" {
		fmt.Println("[hive] GITHUB_PERSONAL_ACCESS_TOKEN injected for MCP server")
	}
}

func appendCertArgs(args []string) []string {
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
	return args
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
	return "HIVE_BEADS_VERSION=" + hiveConfigValDefault("HIVE_BEADS_VERSION", "", defaultBeadsVer)
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
