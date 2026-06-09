package cmd

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestExtractBuildContextCopiesEmbeddedImages(t *testing.T) {
	dir, cleanup, err := extractBuildContext()
	if err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{
		"base/Containerfile",
		"base/hive-agent-entrypoint",
		"claude/Containerfile",
		"copilot/Containerfile",
		"gemini/Containerfile",
		"codex/Containerfile",
	} {
		path := filepath.Join(dir, rel)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected extracted file %s: %v", rel, err)
		}
		if info.IsDir() {
			t.Fatalf("expected %s to be a file", rel)
		}
	}

	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("cleanup should remove build context, stat err=%v", err)
	}
}

func TestEmbeddedContainerfileCopySourcesExist(t *testing.T) {
	containerfiles, err := filepath.Glob(filepath.Join("..", "internal", "imgfs", "images", "*", "Containerfile"))
	if err != nil {
		t.Fatal(err)
	}
	for _, containerfile := range containerfiles {
		data, err := os.ReadFile(containerfile)
		if err != nil {
			t.Fatal(err)
		}
		contextDir := filepath.Dir(containerfile)
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(strings.TrimSpace(line))
			if len(fields) < 2 || fields[0] != "COPY" || strings.HasPrefix(fields[1], "--from=") {
				continue
			}
			source := filepath.Join(contextDir, fields[1])
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("%s copies missing source %s: %v", containerfile, fields[1], err)
			}
		}
	}
}

func TestEmbeddedAgentEntrypointShellSyntax(t *testing.T) {
	cmd := exec.Command("bash", "-n", filepath.Join("..", "internal", "imgfs", "images", "base", "hive-agent-entrypoint"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash -n hive-agent-entrypoint failed: %v\n%s", err, out)
	}
}

func TestExtractBuildContextReturnsWalkError(t *testing.T) {
	_, cleanup, err := extractBuildContextFromFS(fstest.MapFS{})
	if err == nil {
		t.Fatal("extractBuildContextFromFS should fail when images root is missing")
	}
	if cleanup != nil {
		t.Fatal("cleanup should be nil after failed extraction")
	}
	if !strings.Contains(err.Error(), "images") {
		t.Fatalf("error = %v, want images path", err)
	}
}

type readFailFS struct {
	fstest.MapFS
}

func (f readFailFS) Open(name string) (fs.File, error) {
	return f.MapFS.Open(name)
}

func (f readFailFS) ReadFile(name string) ([]byte, error) {
	if name == "images/base/Containerfile" {
		return nil, fs.ErrPermission
	}
	return f.MapFS.ReadFile(name)
}

func TestExtractBuildContextReturnsReadError(t *testing.T) {
	fsys := readFailFS{MapFS: fstest.MapFS{
		"images/base/Containerfile": {Data: []byte("FROM scratch\n")},
	}}
	_, cleanup, err := extractBuildContextFromFS(fsys)
	if err == nil {
		t.Fatal("extractBuildContextFromFS should fail when embedded read fails")
	}
	if cleanup != nil {
		t.Fatal("cleanup should be nil after failed extraction")
	}
}
