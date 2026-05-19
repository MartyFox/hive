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
var modelOverride string

var runCmd = &cobra.Command{
	Use:   "run <agent>",
	Short: "Run an agent sandbox in the current directory",
	Long: `Run an AI coding agent in an isolated Podman container.

Image resolution order:
  1. Local image hive-<agent> exists -> use it
  2. Pull from ghcr.io/martyfox/hive-<agent>:latest -> tag locally -> use it
  3. Pull failed -> build from embedded Containerfiles -> use it

The current directory is mounted read-write at /workspace inside the container.
The agent global config directory (~/.claude/, ~/.copilot/, etc.) is mounted
read-write so auth and personal instructions persist across sessions.`,
	Args: cobra.ExactArgs(1),
	RunE: runRun,
	Example: `  hive run claude
  hive run copilot
  hive run claude --cmd "fix the auth bug"
  hive run copilot --model gpt-5.4
  hive run claude --prompt "write tests" --model claude-opus-4.7`,
}

func init() {
	runCmd.Flags().StringVar(&cmdOverride, "cmd", "", "Run a one-shot command instead of interactive REPL")
	runCmd.Flags().StringVar(&promptOverride, "prompt", "", "Pass a prompt to the agent non-interactively (copilot, claude)")
	runCmd.Flags().StringVar(&modelOverride, "model", "", "Override the model for this session (copilot, claude only)")
}

func runRun(_ *cobra.Command, args []string) error {
	agent := args[0]
	if !podman.ValidAgent(agent) {
		return fmt.Errorf("unknown agent %q - valid:%s", agent, joinAgents())
	}

	// --model validation: trim, restrict to supported agents, reject with --cmd.
	if modelOverride != "" {
		modelOverride = strings.TrimSpace(modelOverride)
		if modelOverride == "" {
			return fmt.Errorf("--model value must not be blank")
		}
		if agent != "claude" && agent != "copilot" {
			return fmt.Errorf("--model not supported for agent %q; supported: claude, copilot", agent)
		}
		if cmdOverride != "" {
			return fmt.Errorf("--model cannot be combined with --cmd; embed --model in the command string directly")
		}
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
	fmt.Printf("[hive] Workspace -> /workspace  (%s)\n", wd)

	// --prompt builds the one-shot command via buildPromptCmd (includes optional --model).
	if promptOverride != "" {
		pc, err := buildPromptCmd(agent, promptOverride, modelOverride)
		if err != nil {
			return err
		}
		cmdOverride = pc
	}

	// Build podman run args (before image name).
	// cleanupSecrets removes the temp env-file holding GH_TOKEN; call after
	// the container exits. For syscall.Exec the cleanup cannot run -- the file
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
		runErr := cmd.Run()
		// If --model was specified and the run failed, show available models as a hint.
		if runErr != nil && modelOverride != "" {
			showAvailableModels(agent, image, modelOverride, runArgs)
		}
		return runErr
	}

	// Interactive: replace this process with podman (proper TTY handoff)
	podmanPath, err := exec.LookPath("podman")
	if err != nil {
		return fmt.Errorf("podman not found: %w", err)
	}
	// Append --model as post-image args; Podman forwards them to the container ENTRYPOINT.
	// e.g. hive-copilot ENTRYPOINT "copilot --yolo" + args "--model gpt-5.4"
	//      -> container runs: copilot --yolo --model gpt-5.4
	if modelOverride != "" && modelPreflightEnabled() {
		if err := preflightModelAvailable(agent, image, modelOverride, runArgs); err != nil {
			cleanupSecrets()
			return err
		}
	}

	imageAndArgs := []string{image}
	if modelOverride != "" {
		imageAndArgs = append(imageAndArgs, "--model", modelOverride)
	}
	allArgs := append([]string{"podman"}, append(runArgs, imageAndArgs...)...)
	return syscall.Exec(podmanPath, allArgs, os.Environ())
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes,
// so it is safe to pass as a shell word inside a bash -c string.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// buildPromptCmd builds the agent-specific one-shot command for a prompt.
// model may be empty (no model override).
// Returns an error for agents that do not support --prompt.
func buildPromptCmd(agent, prompt, model string) (string, error) {
	switch agent {
	case "copilot":
		if model != "" {
			return "copilot --yolo --model " + shellQuote(model) + " --prompt " + shellQuote(prompt), nil
		}
		return "copilot --yolo --prompt " + shellQuote(prompt), nil
	case "claude":
		if model != "" {
			return "claude --dangerously-skip-permissions --model " + shellQuote(model) + " " + shellQuote(prompt), nil
		}
		return "claude --dangerously-skip-permissions " + shellQuote(prompt), nil
	default:
		return "", fmt.Errorf("--prompt not supported for agent %q; use --cmd", agent)
	}
}

// modelsListCmd returns the shell command string to list available models for agent.
// Returns "" if the agent does not support model listing via this mechanism.
// Copilot has no 'models' subcommand; a prompt query is used instead.
// Claude uses the 'models' subcommand.
func modelsListCmd(agent string) string {
	switch agent {
	case "copilot":
		return "copilot --yolo --prompt " + shellQuote("list available agent models")
	case "claude":
		return "claude --dangerously-skip-permissions models"
	default:
		return ""
	}
}

// showAvailableModels runs a secondary container to list available models and
// prints the result to stderr. Called after a --prompt run fails with --model set.
// runArgs is the already-built podman run arg slice (before image name) from the
// failing invocation; it is reused so auth mounts are consistent.
func showAvailableModels(agent, image, model string, runArgs []string) {
	listCmd := modelsListCmd(agent)
	if listCmd == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "\n[hive] model %q not recognized or unavailable -- querying available models for %s:\n", model, agent)
	allArgs := append(runArgs, "--entrypoint", "bash", image, "-c", listCmd)
	cmd := exec.Command("podman", allArgs...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
}

// preflightModelAvailable validates model against model listing before
// interactive launch (syscall.Exec cannot recover to print hints on failure).
// If model listing fails, this check is skipped.
func preflightModelAvailable(agent, image, model string, runArgs []string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}

	listCmd := modelsListCmd(agent)
	if listCmd == "" {
		return nil
	}

	allArgs := append(runArgs, "--entrypoint", "bash", image, "-c", listCmd)
	cmd := exec.Command("podman", allArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[hive warn] unable to preflight model availability for %s: %v\n", agent, err)
		return nil
	}

	output := string(out)
	if modelAppearsInList(output, model) {
		return nil
	}

	fmt.Fprintf(os.Stderr, "[hive] model %q not recognized for %s. Available models:\n%s\n", model, agent, output)
	return fmt.Errorf("model %q not recognized for %s", model, agent)
}

func modelAppearsInList(output, model string) bool {
	model = strings.TrimSpace(strings.ToLower(model))
	if model == "" {
		return true
	}
	return strings.Contains(strings.ToLower(output), model)
}

// modelPreflightEnabled controls interactive preflight model validation.
// Disabled by default for startup performance; enable with HIVE_MODEL_PREFLIGHT=1.
func modelPreflightEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(podman.ConfigValDefault("HIVE_MODEL_PREFLIGHT", "0")))
	return v == "1" || v == "true" || v == "yes"
}

// ensureImage resolves the agent image: local -> pull -> build.
func ensureImage(agent string) (string, error) {
	local := "hive-" + agent
	if podman.ImageExists(local) {
		return local, nil
	}

	// Try registry pull
	reg := podman.RegistryName(agent)
	fmt.Printf("[hive] Image %s not found -- pulling %s...\n", local, reg)
	if err := podman.PullImage(reg); err == nil {
		fmt.Printf("[hive] Tagging %s -> %s\n", reg, local)
		if err := podman.TagImage(reg, local); err != nil {
			return "", fmt.Errorf("tagging pulled image: %w", err)
		}
		return local, nil
	}

	// Fallback: build from embedded
	fmt.Printf("[hive] Pull failed -- building from embedded Containerfiles...\n")
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
