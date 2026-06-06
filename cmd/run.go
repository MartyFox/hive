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
var writableConfig bool
var injectGHToken bool

var (
	imageExistsFunc         = podman.ImageExists
	registryNameFunc        = podman.RegistryName
	pullImageFunc           = podman.PullImage
	tagImageFunc            = podman.TagImage
	extractBuildContextFunc = extractBuildContext
	buildAgentForRunFunc    = buildAgent
)

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
read-only by default. Use --writable-config only when a login or setup flow must
change host config. GitHub token injection is opt-in via --gh-token.`,
	Args: cobra.ExactArgs(1),
	RunE: runRun,
	Example: `  hive run claude
  hive run copilot
  hive run claude --cmd "fix the auth bug"
  hive run copilot --gh-token --prompt "open a PR"`,
}

func init() {
	runCmd.Flags().StringVar(&cmdOverride, "cmd", "", "Run a one-shot command instead of interactive REPL")
	runCmd.Flags().StringVar(&promptOverride, "prompt", "", "Pass a prompt to the agent non-interactively (copilot, claude)")
	runCmd.Flags().BoolVar(&writableConfig, "writable-config", false, "Mount host agent config read-write instead of read-only")
	runCmd.Flags().BoolVar(&injectGHToken, "gh-token", false, "Inject host gh auth token into the container")
}

func runRun(_ *cobra.Command, args []string) error {
	agent := args[0]
	if !podman.ValidAgent(agent) {
		return fmt.Errorf("unknown agent %q — valid:%s", agent, podman.JoinAgents())
	}
	if err := setupAgentRun(agent); err != nil {
		return err
	}
	image, wd, err := prepareRun(agent)
	if err != nil {
		return err
	}
	opts := runOptions()
	if promptOverride != "" {
		return executePromptRun(agent, image, opts)
	}
	runArgs, cleanupSecrets, err := podman.BuildRunArgs(agent, opts)
	if err != nil {
		return err
	}
	if cmdOverride != "" {
		return executeCommandRun(runArgs, cleanupSecrets, image, wd)
	}
	if injectGHToken || podman.GitHubTokenEnabled() {
		return runPodmanChild(append(runArgs, image), cleanupSecrets)
	}
	return execPodman(runArgs, image)
}

func runOptions() podman.RunOptions {
	return podman.RunOptions{
		Interactive:    cmdOverride == "" && promptOverride == "",
		WritableConfig: writableConfig,
		GitHubToken:    injectGHToken,
	}
}

func setupAgentRun(agent string) error {
	if err := podman.CheckPodman(); err != nil {
		return err
	}
	if err := podman.EnsureNetwork(); err != nil {
		return fmt.Errorf("creating hive network: %w", err)
	}
	if agent == "copilot" {
		podman.CleanCopilotMCPConfig()
	}
	return nil
}

func prepareRun(agent string) (string, string, error) {
	image, err := ensureImage(agent)
	if err != nil {
		return "", "", fmt.Errorf("preparing image for %s: %w", agent, err)
	}
	wd, _ := os.Getwd()
	fmt.Printf("[hive] Starting %s sandbox\n", agent)
	fmt.Printf("[hive] Workspace → /workspace  (%s)\n", wd)
	return image, wd, nil
}

func executePromptRun(agent, image string, opts podman.RunOptions) error {
	entrypoint, promptArgs, ok := promptEntrypointArgs(agent, promptOverride)
	if !ok {
		return fmt.Errorf("--prompt not supported for agent %q; use --cmd", agent)
	}
	runArgs, cleanupSecrets, err := podman.BuildRunArgs(agent, opts)
	if err != nil {
		return err
	}
	allArgs := append(runArgs, "--entrypoint", entrypoint, image)
	return runPodmanChild(append(allArgs, promptArgs...), cleanupSecrets)
}

func executeCommandRun(runArgs []string, cleanupSecrets func(), image, wd string) error {
	allArgs := append(runArgs, "--entrypoint", "bash", image, "-c", commandShell(wd))
	return runPodmanChild(allArgs, cleanupSecrets)
}

func commandShell(wd string) string {
	if !podman.BeadsEnabled() {
		return cmdOverride
	}
	if _, err := os.Stat(filepath.Join(wd, ".beads")); !os.IsNotExist(err) {
		return cmdOverride
	}
	return "bd init --quiet 2>/dev/null || true && bash -c " + shellQuote(cmdOverride)
}

func runPodmanChild(args []string, cleanup func()) error {
	defer cleanup()
	cmd := exec.Command("podman", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func execPodman(runArgs []string, image string) error {
	podmanPath, err := exec.LookPath("podman")
	if err != nil {
		return fmt.Errorf("podman not found: %w", err)
	}
	allArgs := append([]string{"podman"}, append(runArgs, image)...)
	return syscall.Exec(podmanPath, allArgs, os.Environ())
}

func promptEntrypointArgs(agent, prompt string) (string, []string, bool) {
	switch agent {
	case "copilot":
		return "copilot", []string{"--yolo", "--prompt", prompt}, true
	case "claude":
		return "claude", []string{"--dangerously-skip-permissions", "-p", prompt}, true
	default:
		return "", nil, false
	}
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes,
// so it is safe to pass as a shell word inside a bash -c string.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ensureImage resolves the agent image: local → pull → build.
func ensureImage(agent string) (string, error) {
	local := "hive-" + agent
	if imageExistsFunc(local) {
		return local, nil
	}

	// Try registry pull
	reg := registryNameFunc(agent)
	fmt.Printf("[hive] Image %s not found — pulling %s...\n", local, reg)
	if err := pullImageFunc(reg); err == nil {
		fmt.Printf("[hive] Tagging %s → %s\n", reg, local)
		if err := tagImageFunc(reg, local); err != nil {
			return "", fmt.Errorf("tagging pulled image: %w", err)
		}
		return local, nil
	}

	// Fallback: build from embedded
	fmt.Printf("[hive] Pull failed — building from embedded Containerfiles...\n")
	ctxDir, cleanup, err := extractBuildContextFunc()
	if err != nil {
		return "", fmt.Errorf("extracting embedded Containerfiles: %w", err)
	}
	defer cleanup()

	if err := buildAgentForRunFunc(agent, ctxDir, false); err != nil {
		return "", fmt.Errorf("building hive-%s: %w", agent, err)
	}
	return local, nil
}
