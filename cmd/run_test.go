package cmd

import (
	"testing"

	"github.com/MartyFox/hive/internal/podman"
)

// ── shellQuote ────────────────────────────────────────────────────────────────

func TestShellQuote_simple(t *testing.T) {
	if got := shellQuote("hello world"); got != "'hello world'" {
		t.Errorf("shellQuote = %q, want %q", got, "'hello world'")
	}
}

func TestShellQuote_empty(t *testing.T) {
	if got := shellQuote(""); got != "''" {
		t.Errorf("shellQuote empty = %q, want %q", got, "''")
	}
}

func TestShellQuote_withSingleQuote(t *testing.T) {
	want := `'it'\''s'`
	if got := shellQuote("it's"); got != want {
		t.Errorf("shellQuote = %q, want %q", got, want)
	}
}

// ── modelsListCmd ─────────────────────────────────────────────────────────────

func TestModelsListCmd_copilot(t *testing.T) {
	got := modelsListCmd("copilot")
	if got == "" {
		t.Error("modelsListCmd(copilot) returned empty, want non-empty")
	}
}

func TestModelsListCmd_claude(t *testing.T) {
	got := modelsListCmd("claude")
	if got == "" {
		t.Error("modelsListCmd(claude) returned empty, want non-empty")
	}
}

func TestModelsListCmd_gemini(t *testing.T) {
	if got := modelsListCmd("gemini"); got != "" {
		t.Errorf("modelsListCmd(gemini) = %q, want empty (not supported)", got)
	}
}

func TestModelsListCmd_codex(t *testing.T) {
	if got := modelsListCmd("codex"); got != "" {
		t.Errorf("modelsListCmd(codex) = %q, want empty (not supported)", got)
	}
}

// ── buildPromptCmd ────────────────────────────────────────────────────────────

func TestBuildPromptCmd_copilot_noModel(t *testing.T) {
	got, err := buildPromptCmd("copilot", "refactor auth", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "copilot --yolo --prompt 'refactor auth'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildPromptCmd_copilot_withModel(t *testing.T) {
	got, err := buildPromptCmd("copilot", "refactor auth", "gpt-5.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "copilot --yolo --model 'gpt-5.4' --prompt 'refactor auth'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildPromptCmd_claude_noModel(t *testing.T) {
	got, err := buildPromptCmd("claude", "fix the auth bug", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "claude --dangerously-skip-permissions 'fix the auth bug'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildPromptCmd_claude_withModel(t *testing.T) {
	got, err := buildPromptCmd("claude", "fix the auth bug", "claude-opus-4.7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "claude --dangerously-skip-permissions --model 'claude-opus-4.7' 'fix the auth bug'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildPromptCmd_unsupportedAgent_noModel(t *testing.T) {
	_, err := buildPromptCmd("gemini", "hello", "")
	if err == nil {
		t.Error("expected error for unsupported agent, got nil")
	}
}

func TestBuildPromptCmd_unsupportedAgent_withModel(t *testing.T) {
	_, err := buildPromptCmd("codex", "hello", "gpt-4")
	if err == nil {
		t.Error("expected error for unsupported agent even with model set, got nil")
	}
}

func TestBuildPromptCmd_promptWithSingleQuote(t *testing.T) {
	// Prompts containing single quotes must be safely escaped.
	got, err := buildPromptCmd("copilot", "it's broken", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `copilot --yolo --prompt 'it'\''s broken'`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildPromptCmd_modelWithSingleQuote(t *testing.T) {
	// Defensive: model names should never contain quotes, but handle gracefully.
	got, err := buildPromptCmd("claude", "task", "weird'model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `claude --dangerously-skip-permissions --model 'weird'\''model' 'task'`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// -- modelAppearsInList ------------------------------------------------------

func TestModelAppearsInList_caseInsensitive(t *testing.T) {
	output := "Available: GPT-5.4\nclaude-opus-4.7\n"
	if !modelAppearsInList(output, "gpt-5.4") {
		t.Fatal("expected model to be found case-insensitively")
	}
}

func TestModelAppearsInList_notFound(t *testing.T) {
	output := "Available: gpt-4.1\nclaude-sonnet-4.0\n"
	if modelAppearsInList(output, "gpt-5.4") {
		t.Fatal("expected model to be absent")
	}
}

func TestModelAppearsInList_blankModel(t *testing.T) {
	if !modelAppearsInList("anything", "   ") {
		t.Fatal("blank model should be treated as present")
	}
}

func TestModelPreflightEnabled_defaultOff(t *testing.T) {
	t.Setenv("HIVE_MODEL_PREFLIGHT", "")
	if modelPreflightEnabled() {
		t.Fatal("expected model preflight disabled by default")
	}
}

func TestModelPreflightEnabled_trueValues(t *testing.T) {
	for _, v := range []string{"1", "true", "yes", " TRUE "} {
		t.Setenv("HIVE_MODEL_PREFLIGHT", v)
		if !modelPreflightEnabled() {
			t.Fatalf("expected enabled for value %q", v)
		}
	}
}

func TestModelPreflightEnabled_readsHiveConfigFallback(t *testing.T) {
	// Ensure wrapper is reachable and returns fallback when unset.
	t.Setenv("HIVE_MODEL_PREFLIGHT", "")
	if got := podman.ConfigValDefault("HIVE_MODEL_PREFLIGHT", "0"); got != "0" {
		t.Fatalf("ConfigValDefault fallback = %q, want 0", got)
	}
}
