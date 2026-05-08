package podman

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const (
	Network  = "hive-net"
	Prefix   = "hive"
	Registry = "ghcr.io/martinf"
)

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
	return Registry + "/" + Prefix + "-" + agent + ":latest"
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

// PullImage pulls an image from a registry, streaming output to stdout/stderr.
func PullImage(name string) error {
	cmd := exec.Command("podman", "pull", name)
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
	probe := exec.Command("podman", "network", "inspect", Network)
	probe.Stdout = nil
	probe.Stderr = nil
	if probe.Run() == nil {
		return nil
	}
	create := exec.Command("podman", "network", "create",
		"--driver", "bridge",
		"--label", "hive.managed=true",
		Network,
	)
	create.Stdout = os.Stdout
	create.Stderr = os.Stderr
	return create.Run()
}

// BuildImage runs podman build -t tag contextDir.
func BuildImage(tag, contextDir string, noCache bool) error {
	args := []string{"build", "-t", tag}
	if noCache {
		args = append(args, "--no-cache")
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
func configMounts(agent string) []configMount {
	home, _ := os.UserHomeDir()
	switch agent {
	case "claude":
		return []configMount{
			{home + "/.claude", "/home/agent/.claude", "claude config"},
		}
	case "copilot":
		copilotSrc := home + "/.copilot"
		if v := os.Getenv("COPILOT_HOME"); v != "" {
			copilotSrc = v
		}
		return []configMount{
			{copilotSrc, "/home/agent/.copilot", "copilot config"},
		}
	case "gemini":
		return []configMount{
			{home + "/.gemini", "/home/agent/.gemini", "gemini config"},
		}
	case "codex":
		return []configMount{
			{home + "/.config/openai", "/home/agent/.config/openai", "codex config"},
		}
	}
	return nil
}

// ghToken tries to obtain the current gh CLI token from the host.
// On macOS, gh stores tokens in the Keychain rather than ~/.config/gh/,
// so we call "gh auth token" and inject the result as GH_TOKEN into the container.
// gh inside the container honours GH_TOKEN, so github-mcp-server's "gh auth token"
// call will succeed without needing any file mount.
func ghToken() string {
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// BuildRunArgs builds the podman run argument slice (after "podman", before image name).
// interactive should be true for REPL sessions (adds -it).
func BuildRunArgs(agent string, interactive bool) []string {
	wd, _ := os.Getwd()

	args := []string{"run", "--rm"}
	if interactive {
		args = append(args, "-it")
	}
	args = append(args,
		"--cap-drop=ALL",
		"--security-opt", "no-new-privileges",
		"--network", Network,
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

	// Copilot: inject gh auth token so github-mcp-server can call "gh auth token" inside
	// the container. On macOS the token lives in the Keychain, not ~/.config/gh/, so a
	// bind mount would be empty. GH_TOKEN is honoured by gh and by github-mcp-server.
	if agent == "copilot" {
		if tok := ghToken(); tok != "" {
			args = append(args, "-e", "GH_TOKEN="+tok)
			fmt.Println("[hive] GH_TOKEN injected for github-mcp-server")
		} else {
			fmt.Fprintln(os.Stderr,
				"[hive warn] gh auth token not found — github-mcp-server will be unauthenticated\n"+
					"  Fix: run 'gh auth login' on the host, then retry")
		}
	}

	return args
}

// JoinAgents returns a space-joined string of all agent names.
func JoinAgents() string {
	return " " + strings.Join(agents, " ")
}
