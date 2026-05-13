package cmd

import (
	"fmt"
	"os"

	"github.com/martinf/hive/internal/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "hive",
	Short: "Host Isolated Virtual Environment — AI agent sandbox",
	Long: fmt.Sprintf(`hive %s

Runs AI coding agents (Claude, Copilot, Gemini, Codex) inside
isolated Podman containers with read-write access to your project only.`, version.String()),
	SilenceUsage: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(versionCmd)
}
