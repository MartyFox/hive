package cmd

import (
	"fmt"

	"github.com/MartyFox/hive/internal/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print hive version information",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("hive", version.String())
	},
}
