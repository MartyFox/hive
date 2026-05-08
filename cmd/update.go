package cmd

import (
	"fmt"

	"github.com/martinf/hive/internal/podman"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update [agent|base|all]",
	Short: "Rebuild image(s) without cache (picks up latest CLI versions)",
	Long: `Rebuild one or all hive images from embedded Containerfiles using --no-cache.
This forces npm install to fetch the latest published CLI versions.`,
	Args:    cobra.MaximumNArgs(1),
	RunE:    runUpdate,
	Example: `  hive update
  hive update claude`,
}

func runUpdate(_ *cobra.Command, args []string) error {
	target := "all"
	if len(args) > 0 {
		target = args[0]
	}

	if err := podman.CheckPodman(); err != nil {
		return err
	}

	ctxDir, cleanup, err := extractBuildContext()
	if err != nil {
		return fmt.Errorf("extracting embedded Containerfiles: %w", err)
	}
	defer cleanup()

	switch target {
	case "all":
		if err := buildBase(ctxDir, true); err != nil {
			return err
		}
		for _, a := range podman.Agents() {
			if err := buildAgent(a, ctxDir, true); err != nil {
				return err
			}
		}
		fmt.Println("[hive] All images updated.")
	case "base":
		return buildBase(ctxDir, true)
	default:
		if !podman.ValidAgent(target) {
			return fmt.Errorf("unknown agent %q — valid: base %s", target, joinAgents())
		}
		return buildAgent(target, ctxDir, true)
	}
	return nil
}
