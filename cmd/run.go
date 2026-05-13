package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/MartyFox/hive/internal/podman"
	"github.com/spf13/cobra"
)

var cmdOverride string
var promptOverride string

var runCmd = &cobra.Command{
	Use:   "run <agent>",
	Short: "Run an agent sandbox in the current directory",
	Long: `Run an AI coding agent in an isolated Podman container.

Image resolution order:
  1. Local image hive-<agent> exists → use it
	2. Pull from ghcr.io/martyfox/hive-<agent>:latest → tag locally → use it
  3. Pull failed → build from embedded Containerfiles → use it

The current directory is mounted read-write at /workspace inside the container.
The agent's global config directory (~/.claude/, ~/.copilot/, etc.) is mounted
read-write so auth and personal instructions persist across sessions.`,
	Args:    cobra.ExactArgs(1),
	RunE:    runRun,
	Example: `  hive run claude
  hive run copilot
  hive run claude --cmd "fix the auth bug"`,
}

func init() {
	runCmd.Flags().StringVar(&cmdOverride, "cmd", "", "Run a one-shot command instead of interactive REPL")
	runCmd.Flags().StringVar(&promptOverride, "prompt", "", "Pass a prompt to the agent non-interactively (copilot, claude)")
}

func runRun(_ *cobra.Command, args []string) error {
	agent := args[0]
	if !podman.ValidAgent(agent) {
		return fmt.Errorf("unknown agent %q — valid:%s", agent, joinAgents())
	}

	if err := podman.CheckPodman(); err != nil {
		return err
	}
	if err := podman.EnsureNetwork(); err != nil {
		return fmt.Errorf("creating hive network: %w", err)
	}

	// Copilot: remove any hive-generated mcp-config.json that conflicts with the
	// built-in remote MCP transport (v1.0.44+ uses SSE, no local binary needed).
	if agent == "copilot" {
		podman.CleanCopilotMCPConfig()
	}

	image, err := ensureImage(agent)
	if err != nil {
		return fmt.Errorf("preparing image for %s: %w", agent, err)
	}

	wd, _ := os.Getwd()
	fmt.Printf("[hive] Starting %s sandbox\n", agent)
	fmt.Printf("[hive] Workspace → /workspace  (%s)\n", wd)

	// --prompt is sugar over --cmd with agent-specific prompt flag syntax.
	if promptOverride != "" {
		switch agent {
		case "copilot":
			cmdOverride = fmt.Sprintf("copilot --yolo --prompt %q", promptOverride)
		case "claude":
			cmdOverride = fmt.Sprintf("claude --dangerously-skip-permissions -p %q", promptOverride)
		default:
			return fmt.Errorf("--prompt not supported for agent %q; use --cmd", agent)
		}
	}

	// Build podman run args (before image name).
	// cleanupSecrets removes the temp env-file holding GH_TOKEN; call after
	// the container exits. For syscall.Exec the cleanup cannot run — the file
	// lingers in /tmp (0600) and is cleaned by the OS.
	runArgs, cleanupSecrets := podman.BuildRunArgs(agent, cmdOverride == "")

	if cmdOverride != "" {
		defer cleanupSecrets()
		// Non-interactive one-shot
		beadsPrelude := ""
		if podman.BeadsEnabled() {
			if _, err := os.Stat(filepath.Join(wd, ".beads")); os.IsNotExist(err) {
				beadsPrelude = "bd init --quiet 2>/dev/null || true && "
			}
		}
		// When a beads prelude is prepended, shell-quote the user command and
		// run it via a nested bash -c to prevent metacharacter injection from
		// cmdOverride into the prelude.
		var shellCmd string
		if beadsPrelude != "" {
			shellCmd = beadsPrelude + "bash -c " + shellQuote(cmdOverride)
		} else {
			shellCmd = cmdOverride
		}
		allArgs := append(runArgs, "--entrypoint", "bash", image, "-c", shellCmd)
		cmd := exec.Command("podman", allArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Interactive: replace this process with podman (proper TTY handoff)
	podmanPath, err := exec.LookPath("podman")
	if err != nil {
		return fmt.Errorf("podman not found: %w", err)
	}
	allArgs := append([]string{"podman"}, append(runArgs, image)...)
	return syscall.Exec(podmanPath, allArgs, os.Environ())
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes,
// so it is safe to pass as a shell word inside a bash -c string.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ensureImage resolves the agent image: local → pull → build.
func ensureImage(agent string) (string, error) {
	local := "hive-" + agent
	if podman.ImageExists(local) {
		return local, nil
	}

	// Try registry pull
	reg := podman.RegistryName(agent)
	fmt.Printf("[hive] Image %s not found — pulling %s...\n", local, reg)
	if err := podman.PullImage(reg); err == nil {
		fmt.Printf("[hive] Tagging %s → %s\n", reg, local)
		if err := podman.TagImage(reg, local); err != nil {
			return "", fmt.Errorf("tagging pulled image: %w", err)
		}
		return local, nil
	}

	// Fallback: build from embedded
	fmt.Printf("[hive] Pull failed — building from embedded Containerfiles...\n")
	ctxDir, cleanup, err := extractBuildContext()
	if err != nil {
		return "", fmt.Errorf("extracting embedded Containerfiles: %w", err)
	}
	defer cleanup()

	if err := buildAgent(agent, ctxDir, false); err != nil {
		return "", fmt.Errorf("building hive-%s: %w", agent, err)
	}
	return local, nil
}
