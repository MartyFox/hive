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
	Args: cobra.MaximumNArgs(1),
	RunE: runBuild,
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
	return buildTarget(target, ctxDir, false)
}

func buildTarget(target, ctxDir string, noCache bool) error {
	switch target {
	case "all":
		return buildAll(ctxDir, noCache)
	case "base":
		return buildBase(ctxDir, noCache)
	default:
		return buildSingleAgent(target, ctxDir, noCache)
	}
}

func buildAll(ctxDir string, noCache bool) error {
	if err := buildBase(ctxDir, noCache); err != nil {
		return err
	}
	for _, a := range podman.Agents() {
		if err := buildAgent(a, ctxDir, noCache); err != nil {
			return err
		}
	}
	action := "built"
	if noCache {
		action = "updated"
	}
	fmt.Printf("[hive] All images %s.\n", action)
	return nil
}

func buildSingleAgent(target, ctxDir string, noCache bool) error {
	if !podman.ValidAgent(target) {
		return fmt.Errorf("unknown agent %q — valid: base %s", target, podman.JoinAgents())
	}
	return buildAgent(target, ctxDir, noCache)
}

func buildBase(ctxDir string, noCache bool) error {
	fmt.Println("[hive] Building hive-base...")
	baseCtx := filepath.Join(ctxDir, "base")
	if err := podman.InjectCertToContext(baseCtx); err != nil {
		return fmt.Errorf("injecting cert into build context: %w", err)
	}
	return podman.BuildImage("hive-base", baseCtx, noCache, []string{podman.BeadsArg(), podman.BeadsVersionArg()})
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
	return extractBuildContextFromFS(imgfs.FS)
}

func extractBuildContextFromFS(fsys fs.FS) (dir string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", "hive-build-*")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { os.RemoveAll(dir) }

	err = fs.WalkDir(fsys, "images", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		return extractBuildEntry(fsys, dir, path, d)
	})
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return dir, cleanup, nil
}

func extractBuildEntry(fsys fs.FS, dir, path string, d fs.DirEntry) error {
	rel, err := filepath.Rel("images", path)
	if err != nil {
		return fmt.Errorf("unexpected embedded path %s: %w", path, err)
	}
	dest := filepath.Join(dir, rel)
	if d.IsDir() {
		return os.MkdirAll(dest, 0755)
	}
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0644)
}

