package cmd

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestPromptEntrypointArgsDoesNotUseShell(t *testing.T) {
	prompt := "$(touch /workspace/pwned); echo 'hello'"

	entrypoint, args, ok := promptEntrypointArgs("claude", prompt)
	if !ok {
		t.Fatal("promptEntrypointArgs() ok = false, want true")
	}
	if entrypoint != "claude" {
		t.Fatalf("entrypoint = %q, want claude", entrypoint)
	}
	want := []string{"--dangerously-skip-permissions", "-p", prompt}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	for _, arg := range args {
		if arg == "bash" || arg == "-c" {
			t.Fatalf("prompt args should not invoke shell: %#v", args)
		}
	}
}

func TestPromptEntrypointArgsRejectsUnsupportedAgent(t *testing.T) {
	if _, _, ok := promptEntrypointArgs("gemini", "hello"); ok {
		t.Fatal("promptEntrypointArgs() ok = true for unsupported agent")
	}
}

func TestRunRejectsUnknownAgentBeforePodman(t *testing.T) {
	err := runRun(nil, []string{"unknown"})
	if err == nil {
		t.Fatal("runRun should reject unknown agent")
	}
	if !strings.Contains(err.Error(), `unknown agent "unknown"`) {
		t.Fatalf("runRun error = %v, want unknown agent", err)
	}
	if !strings.Contains(err.Error(), "claude") || !strings.Contains(err.Error(), "codex") {
		t.Fatalf("runRun error = %v, want valid agent list", err)
	}
}

func resetEnsureImageSeams(t *testing.T) {
	t.Helper()
	oldImageExists := imageExistsFunc
	oldRegistryName := registryNameFunc
	oldPullImage := pullImageFunc
	oldTagImage := tagImageFunc
	oldExtractBuildContext := extractBuildContextFunc
	oldBuildAgent := buildAgentForRunFunc
	t.Cleanup(func() {
		imageExistsFunc = oldImageExists
		registryNameFunc = oldRegistryName
		pullImageFunc = oldPullImage
		tagImageFunc = oldTagImage
		extractBuildContextFunc = oldExtractBuildContext
		buildAgentForRunFunc = oldBuildAgent
	})
}

func TestEnsureImageUsesLocalImage(t *testing.T) {
	resetEnsureImageSeams(t)
	calledPull := false
	imageExistsFunc = func(name string) bool { return name == "hive-claude" }
	pullImageFunc = func(name string) error {
		calledPull = true
		return nil
	}

	got, err := ensureImage("claude")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hive-claude" {
		t.Fatalf("ensureImage() = %q, want hive-claude", got)
	}
	if calledPull {
		t.Fatal("ensureImage should not pull when local image exists")
	}
}

func TestEnsureImagePullsAndTagsRegistryImage(t *testing.T) {
	resetEnsureImageSeams(t)
	imageExistsFunc = func(name string) bool { return false }
	registryNameFunc = func(agent string) string { return "registry/hive-" + agent + ":latest" }
	pullImageFunc = func(name string) error {
		if name != "registry/hive-copilot:latest" {
			t.Fatalf("pull image = %q, want registry/hive-copilot:latest", name)
		}
		return nil
	}
	tagImageFunc = func(src, dst string) error {
		if src != "registry/hive-copilot:latest" || dst != "hive-copilot" {
			t.Fatalf("tag image src=%q dst=%q", src, dst)
		}
		return nil
	}

	got, err := ensureImage("copilot")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hive-copilot" {
		t.Fatalf("ensureImage() = %q, want hive-copilot", got)
	}
}

func TestEnsureImageFallsBackToBuild(t *testing.T) {
	resetEnsureImageSeams(t)
	cleaned := false
	imageExistsFunc = func(name string) bool { return false }
	registryNameFunc = func(agent string) string { return "registry/hive-" + agent + ":latest" }
	pullImageFunc = func(name string) error { return errors.New("pull failed") }
	extractBuildContextFunc = func() (string, func(), error) {
		return "/tmp/hive-build-test", func() { cleaned = true }, nil
	}
	buildAgentForRunFunc = func(agent, ctxDir string, noCache bool) error {
		if agent != "gemini" || ctxDir != "/tmp/hive-build-test" || noCache {
			t.Fatalf("buildAgent args agent=%q ctxDir=%q noCache=%v", agent, ctxDir, noCache)
		}
		return nil
	}

	got, err := ensureImage("gemini")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hive-gemini" {
		t.Fatalf("ensureImage() = %q, want hive-gemini", got)
	}
	if !cleaned {
		t.Fatal("ensureImage should cleanup extracted build context")
	}
}

func TestEnsureImageReturnsTagError(t *testing.T) {
	resetEnsureImageSeams(t)
	imageExistsFunc = func(name string) bool { return false }
	registryNameFunc = func(agent string) string { return "registry/hive-" + agent + ":latest" }
	pullImageFunc = func(name string) error { return nil }
	tagImageFunc = func(src, dst string) error { return errors.New("tag failed") }

	_, err := ensureImage("codex")
	if err == nil {
		t.Fatal("ensureImage should return tag error")
	}
	if !strings.Contains(err.Error(), "tagging pulled image: tag failed") {
		t.Fatalf("ensureImage error = %v, want tag failure", err)
	}
}
