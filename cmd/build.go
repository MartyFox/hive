package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/MartyFox/hive/internal/imgfs"
	"github.com/MartyFox/hive/internal/podman"
	"github.com/spf13/cobra"
)

var buildCmd = &cobra.Command{
	Use:   "build [agent|base|all]",
	Short: "Build hive image(s) from embedded Containerfiles",
	Long: `Build one or all hive images from the Containerfiles embedded in this binary.
If no argument is given, all images are built (base first, then all agents).`,
	Args:    cobra.MaximumNArgs(1),
	RunE:    runBuild,
	Example: `  hive build
  hive build claude
  hive build base`,
}

func runBuild(cmd *cobra.Command, args []string) error {
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
		if err := buildBase(ctxDir, false); err != nil {
			return err
		}
		for _, a := range podman.Agents() {
			if err := buildAgent(a, ctxDir, false); err != nil {
				return err
			}
		}
		fmt.Println("[hive] All images built.")
	case "base":
		return buildBase(ctxDir, false)
	default:
		if !podman.ValidAgent(target) {
			return fmt.Errorf("unknown agent %q — valid: base %s", target, joinAgents())
		}
		return buildAgent(target, ctxDir, false)
	}
	return nil
}

func buildBase(ctxDir string, noCache bool) error {
	fmt.Println("[hive] Building hive-base...")
	baseCtx := filepath.Join(ctxDir, "base")
	if err := podman.InjectCertToContext(baseCtx); err != nil {
		return fmt.Errorf("injecting cert into build context: %w", err)
	}
	return podman.BuildImage("hive-base", baseCtx, noCache, []string{podman.BeadsArg()})
}

func buildAgent(agent, ctxDir string, noCache bool) error {
	if !podman.ImageExists("hive-base") {
		if err := buildBase(ctxDir, noCache); err != nil {
			return err
		}
	}
	fmt.Printf("[hive] Building hive-%s...\n", agent)
	return podman.BuildImage("hive-"+agent, filepath.Join(ctxDir, agent), noCache, nil)
}

// extractBuildContext extracts the embedded images/* tree to a temp directory.
func extractBuildContext() (dir string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", "hive-build-*")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { os.RemoveAll(dir) }

	err = fs.WalkDir(imgfs.FS, "images", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// path is like "images/base/Containerfile"
		// strip leading "images/" to get "base/Containerfile"
		rel, err := filepath.Rel("images", path)
		if err != nil {
			return fmt.Errorf("unexpected embedded path %s: %w", path, err)
		}
		dest := filepath.Join(dir, rel)

		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		data, err := imgfs.FS.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0644)
	})
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return dir, cleanup, nil
}

func joinAgents() string {
	s := ""
	for _, a := range podman.Agents() {
		s += " " + a
	}
	return s
}
