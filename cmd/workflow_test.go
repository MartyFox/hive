package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MartyFox/hive/internal/podman"
)

func TestBuildImagesWorkflowMatchesSupportedAgents(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "build-images.yml"))
	if err != nil {
		t.Fatal(err)
	}
	agents := workflowAgents(string(data))
	if !reflect.DeepEqual(agents, podman.Agents()) {
		t.Fatalf("workflow agents = %#v, want %#v", agents, podman.Agents())
	}
	for _, dir := range append([]string{"base"}, podman.Agents()...) {
		path := filepath.Join("..", "internal", "imgfs", "images", dir, "Containerfile")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("workflow image %q missing embedded Containerfile: %v", dir, err)
		}
	}
}

func workflowAgents(workflow string) []string {
	for _, line := range strings.Split(workflow, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "for agent in ") {
			continue
		}
		line = strings.TrimPrefix(line, "for agent in ")
		line = strings.TrimSuffix(line, "; do")
		return strings.Fields(line)
	}
	return nil
}
