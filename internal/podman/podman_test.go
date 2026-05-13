package podman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// setHome redirects os.UserHomeDir() to a temp directory for the duration of
// the test. Returns the temp dir path.
func setHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

// writeHiveConfig writes content to <home>/.hive/config.
func writeHiveConfig(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".hive")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// ── ValidAgent ────────────────────────────────────────────────────────────────

func TestValidAgent_knownAgents(t *testing.T) {
	for _, a := range []string{"claude", "copilot", "gemini", "codex"} {
		if !ValidAgent(a) {
			t.Errorf("ValidAgent(%q) = false, want true", a)
		}
	}
}

func TestValidAgent_unknown(t *testing.T) {
	for _, a := range []string{"", "gpt", "llama", "CLAUDE"} {
		if ValidAgent(a) {
			t.Errorf("ValidAgent(%q) = true, want false", a)
		}
	}
}

// ── Agents ────────────────────────────────────────────────────────────────────

func TestAgents_returnsCopy(t *testing.T) {
	a := Agents()
	if len(a) == 0 {
		t.Fatal("Agents() returned empty slice")
	}
	// Mutating the returned slice must not affect the original.
	a[0] = "mutated"
	if ValidAgent("mutated") {
		t.Error("Agents() returned reference to internal slice")
	}
}

// ── RegistryName ──────────────────────────────────────────────────────────────

func TestRegistryName_default(t *testing.T) {
	setHome(t) // isolate from any real ~/.hive/config
	t.Setenv("HIVE_REGISTRY", "")

	got := RegistryName("claude")
	want := "ghcr.io/martyfox/hive-claude:latest"
	if got != want {
		t.Errorf("RegistryName(claude) = %q, want %q", got, want)
	}
}

func TestRegistryName_envOverride(t *testing.T) {
	setHome(t)
	t.Setenv("HIVE_REGISTRY", "ghcr.io/myorg")

	got := RegistryName("copilot")
	want := "ghcr.io/myorg/hive-copilot:latest"
	if got != want {
		t.Errorf("RegistryName(copilot) = %q, want %q", got, want)
	}
}

// ── Network ───────────────────────────────────────────────────────────────────

func TestNetwork_default(t *testing.T) {
	setHome(t)
	t.Setenv("HIVE_NETWORK", "")

	if got := Network(); got != defaultNetwork {
		t.Errorf("Network() = %q, want %q", got, defaultNetwork)
	}
}

func TestNetwork_envOverride(t *testing.T) {
	setHome(t)
	t.Setenv("HIVE_NETWORK", "custom-net")

	if got := Network(); got != "custom-net" {
		t.Errorf("Network() = %q, want custom-net", got)
	}
}

// ── hiveConfig ────────────────────────────────────────────────────────────────

func TestHiveConfig_missingFile(t *testing.T) {
	setHome(t) // temp home with no .hive/config

	cfg := hiveConfig()
	if len(cfg) != 0 {
		t.Errorf("hiveConfig() with missing file = %v, want empty map", cfg)
	}
}

func TestHiveConfig_parsesKeyValue(t *testing.T) {
	home := setHome(t)
	writeHiveConfig(t, home, "FOO=bar\nBAZ=qux\n")

	cfg := hiveConfig()
	if cfg["FOO"] != "bar" {
		t.Errorf("FOO = %q, want bar", cfg["FOO"])
	}
	if cfg["BAZ"] != "qux" {
		t.Errorf("BAZ = %q, want qux", cfg["BAZ"])
	}
}

func TestHiveConfig_ignoresComments(t *testing.T) {
	home := setHome(t)
	writeHiveConfig(t, home, "# this is a comment\nKEY=value\n")

	cfg := hiveConfig()
	if _, ok := cfg["# this is a comment"]; ok {
		t.Error("hiveConfig() should not parse comment lines as keys")
	}
	if cfg["KEY"] != "value" {
		t.Errorf("KEY = %q, want value", cfg["KEY"])
	}
}

func TestHiveConfig_ignoresBlankLines(t *testing.T) {
	home := setHome(t)
	writeHiveConfig(t, home, "\n\nKEY=value\n\n")

	cfg := hiveConfig()
	if cfg["KEY"] != "value" {
		t.Errorf("KEY = %q, want value", cfg["KEY"])
	}
	if len(cfg) != 1 {
		t.Errorf("hiveConfig() len = %d, want 1", len(cfg))
	}
}

func TestHiveConfig_ignoresMalformedLines(t *testing.T) {
	home := setHome(t)
	writeHiveConfig(t, home, "notakeyvaluepair\nGOOD=yes\n")

	cfg := hiveConfig()
	if _, ok := cfg["notakeyvaluepair"]; ok {
		t.Error("malformed line should not appear in config map")
	}
	if cfg["GOOD"] != "yes" {
		t.Errorf("GOOD = %q, want yes", cfg["GOOD"])
	}
}

func TestHiveConfig_trimsWhitespace(t *testing.T) {
	home := setHome(t)
	writeHiveConfig(t, home, "  KEY  =  value  \n")

	cfg := hiveConfig()
	if cfg["KEY"] != "value" {
		t.Errorf("KEY = %q, want value (should be trimmed)", cfg["KEY"])
	}
}

func TestHiveConfig_valueWithEquals(t *testing.T) {
	home := setHome(t)
	writeHiveConfig(t, home, "URL=https://example.com?foo=bar\n")

	cfg := hiveConfig()
	// strings.Cut splits on first = only
	if cfg["URL"] != "https://example.com?foo=bar" {
		t.Errorf("URL = %q, want full value with embedded =", cfg["URL"])
	}
}

// ── hiveConfigVal / hiveConfigValDefault ─────────────────────────────────────

func TestHiveConfigVal_envWinsOverFile(t *testing.T) {
	home := setHome(t)
	writeHiveConfig(t, home, "MY_KEY=fromfile\n")
	t.Setenv("MY_KEY", "fromenv")

	if got := hiveConfigVal("MY_KEY"); got != "fromenv" {
		t.Errorf("hiveConfigVal: env should win, got %q", got)
	}
}

func TestHiveConfigVal_fallsBackToFile(t *testing.T) {
	home := setHome(t)
	writeHiveConfig(t, home, "MY_KEY=fromfile\n")
	t.Setenv("MY_KEY", "") // ensure env unset

	if got := hiveConfigVal("MY_KEY"); got != "fromfile" {
		t.Errorf("hiveConfigVal: file fallback, got %q", got)
	}
}

func TestHiveConfigValDefault_usesDefault(t *testing.T) {
	setHome(t)
	t.Setenv("NO_SUCH_KEY", "")

	if got := hiveConfigValDefault("NO_SUCH_KEY", "mydefault"); got != "mydefault" {
		t.Errorf("hiveConfigValDefault: got %q, want mydefault", got)
	}
}

func TestHiveConfigValDefault_envOverridesDefault(t *testing.T) {
	setHome(t)
	t.Setenv("MY_KEY", "envval")

	if got := hiveConfigValDefault("MY_KEY", "default"); got != "envval" {
		t.Errorf("hiveConfigValDefault: got %q, want envval", got)
	}
}

// ── tlsVerifyFlag ─────────────────────────────────────────────────────────────

func TestTlsVerifyFlag_disabled(t *testing.T) {
	setHome(t)
	t.Setenv("HIVE_TLS_VERIFY", "false")

	if got := tlsVerifyFlag(); got != "--tls-verify=false" {
		t.Errorf("tlsVerifyFlag() = %q, want --tls-verify=false", got)
	}
}

func TestTlsVerifyFlag_disabledUpperCase(t *testing.T) {
	setHome(t)
	t.Setenv("HIVE_TLS_VERIFY", "FALSE")

	if got := tlsVerifyFlag(); got != "--tls-verify=false" {
		t.Errorf("tlsVerifyFlag() uppercase = %q, want --tls-verify=false", got)
	}
}

func TestTlsVerifyFlag_unset(t *testing.T) {
	setHome(t)
	t.Setenv("HIVE_TLS_VERIFY", "")

	if got := tlsVerifyFlag(); got != "" {
		t.Errorf("tlsVerifyFlag() unset = %q, want empty", got)
	}
}

func TestTlsVerifyFlag_true(t *testing.T) {
	setHome(t)
	t.Setenv("HIVE_TLS_VERIFY", "true")

	if got := tlsVerifyFlag(); got != "" {
		t.Errorf("tlsVerifyFlag() true = %q, want empty", got)
	}
}

// ── BeadsEnabled ─────────────────────────────────────────────────────────────

func TestBeadsEnabled_opt_in(t *testing.T) {
	setHome(t)
	t.Setenv("HIVE_BEADS", "1")

	if !BeadsEnabled() {
		t.Error("BeadsEnabled() = false, want true when HIVE_BEADS=1")
	}
}

func TestBeadsEnabled_unset(t *testing.T) {
	setHome(t)
	t.Setenv("HIVE_BEADS", "")

	if BeadsEnabled() {
		t.Error("BeadsEnabled() = true, want false when unset")
	}
}

func TestBeadsEnabled_otherValue(t *testing.T) {
	setHome(t)
	t.Setenv("HIVE_BEADS", "yes")

	if BeadsEnabled() {
		t.Error("BeadsEnabled() = true for 'yes', want false (only '1' enables)")
	}
}

// ── hasOtherMCPEntries ────────────────────────────────────────────────────────

func TestHasOtherMCPEntries_hiveOnly(t *testing.T) {
	content := `{
  "mcpServers": {
    "github-mcp-server": {
      "command": "/usr/local/bin/github-mcp-server",
      "args": ["stdio"]
    }
  }
}`
	if hasOtherMCPEntries(content) {
		t.Error("hasOtherMCPEntries() = true for hive-only file, want false")
	}
}

func TestHasOtherMCPEntries_withExtraCommand(t *testing.T) {
	content := `{
  "mcpServers": {
    "github-mcp-server": {
      "command": "/usr/local/bin/github-mcp-server",
      "args": ["stdio"]
    },
    "my-server": {
      "command": "/usr/local/bin/my-server",
      "args": []
    }
  }
}`
	if !hasOtherMCPEntries(content) {
		t.Error("hasOtherMCPEntries() = false but extra command entry exists")
	}
}

func TestHasOtherMCPEntries_withURL(t *testing.T) {
	content := `{
  "mcpServers": {
    "github-mcp-server": {
      "command": "/usr/local/bin/github-mcp-server"
    },
    "remote": {
      "url": "https://example.com/mcp"
    }
  }
}`
	if !hasOtherMCPEntries(content) {
		t.Error("hasOtherMCPEntries() = false but url entry exists")
	}
}

// ── CleanCopilotMCPConfig ─────────────────────────────────────────────────────

func TestCleanCopilotMCPConfig_removesHiveFile(t *testing.T) {
	home := setHome(t)
	copilotDir := filepath.Join(home, ".copilot")
	if err := os.MkdirAll(copilotDir, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COPILOT_HOME", copilotDir)

	mcpPath := filepath.Join(copilotDir, "mcp-config.json")
	content := `{
  "mcpServers": {
    "github-mcp-server": {
      "command": "/usr/local/bin/github-mcp-server",
      "args": ["stdio"]
    }
  }
}`
	if err := os.WriteFile(mcpPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	CleanCopilotMCPConfig()

	if _, err := os.Stat(mcpPath); !os.IsNotExist(err) {
		t.Error("CleanCopilotMCPConfig() should have removed hive-generated file")
	}
}

func TestCleanCopilotMCPConfig_preservesUserFile(t *testing.T) {
	home := setHome(t)
	copilotDir := filepath.Join(home, ".copilot")
	if err := os.MkdirAll(copilotDir, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COPILOT_HOME", copilotDir)

	mcpPath := filepath.Join(copilotDir, "mcp-config.json")
	// User file has the hive entry PLUS a custom entry.
	content := `{
  "mcpServers": {
    "github-mcp-server": {
      "command": "/usr/local/bin/github-mcp-server"
    },
    "my-tool": {
      "command": "/usr/local/bin/my-tool"
    }
  }
}`
	if err := os.WriteFile(mcpPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	CleanCopilotMCPConfig()

	if _, err := os.Stat(mcpPath); err != nil {
		t.Error("CleanCopilotMCPConfig() should NOT remove file with user entries")
	}
}

func TestCleanCopilotMCPConfig_noopWhenAbsent(t *testing.T) {
	home := setHome(t)
	copilotDir := filepath.Join(home, ".copilot")
	if err := os.MkdirAll(copilotDir, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COPILOT_HOME", copilotDir)

	// Should not panic or error when file doesn't exist.
	CleanCopilotMCPConfig()
}

func TestCleanCopilotMCPConfig_noopForUnrelatedFile(t *testing.T) {
	home := setHome(t)
	copilotDir := filepath.Join(home, ".copilot")
	if err := os.MkdirAll(copilotDir, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COPILOT_HOME", copilotDir)

	mcpPath := filepath.Join(copilotDir, "mcp-config.json")
	content := `{"mcpServers":{"my-tool":{"url":"https://example.com/mcp"}}}`
	if err := os.WriteFile(mcpPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	CleanCopilotMCPConfig()

	if _, err := os.Stat(mcpPath); err != nil {
		t.Error("CleanCopilotMCPConfig() should NOT remove file with no github-mcp-server entry")
	}
}

// ── InjectCertToContext ───────────────────────────────────────────────────────

func TestInjectCertToContext_writesCertWhenExists(t *testing.T) {
	home := setHome(t)
	hiveDir := filepath.Join(home, ".hive")
	if err := os.MkdirAll(hiveDir, 0o750); err != nil {
		t.Fatal(err)
	}
	certContent := []byte("-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n")
	if err := os.WriteFile(filepath.Join(hiveDir, "extra-ca.pem"), certContent, 0o600); err != nil {
		t.Fatal(err)
	}

	ctxDir := t.TempDir()
	if err := InjectCertToContext(ctxDir); err != nil {
		t.Fatalf("InjectCertToContext() error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(ctxDir, "extra-ca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(certContent) {
		t.Errorf("injected cert content mismatch")
	}
}

func TestInjectCertToContext_noopWhenNoCert(t *testing.T) {
	setHome(t) // temp home with no .hive/extra-ca.pem

	ctxDir := t.TempDir()
	if err := InjectCertToContext(ctxDir); err != nil {
		t.Fatalf("InjectCertToContext() error: %v", err)
	}

	_, err := os.Stat(filepath.Join(ctxDir, "extra-ca.pem"))
	if !os.IsNotExist(err) {
		t.Errorf("extra-ca.pem should not exist when no cert configured; err=%v", err)
	}
}

// ── BuildRunArgs ──────────────────────────────────────────────────────────────

func TestBuildRunArgs_alwaysHasSecurityFlags(t *testing.T) {
	setHome(t)
	t.Setenv("HIVE_NETWORK", "test-net")

	args, cleanup := BuildRunArgs("claude", false)
	defer cleanup()
	joined := strings.Join(args, " ")

	for _, want := range []string{"--cap-drop=ALL", "--security-opt", "no-new-privileges", "--network", "test-net"} {
		if !strings.Contains(joined, want) {
			t.Errorf("BuildRunArgs missing %q in %q", want, joined)
		}
	}
}

func TestBuildRunArgs_interactiveAddsIT(t *testing.T) {
	setHome(t)

	args, cleanup := BuildRunArgs("claude", true)
	defer cleanup()
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-it") {
		t.Errorf("BuildRunArgs interactive missing -it in %q", joined)
	}
}

func TestBuildRunArgs_nonInteractiveNoIT(t *testing.T) {
	setHome(t)

	args, cleanup := BuildRunArgs("claude", false)
	defer cleanup()
	for _, a := range args {
		if a == "-it" {
			t.Error("BuildRunArgs non-interactive should not include -it")
		}
	}
}

func TestBuildRunArgs_workspaceMount(t *testing.T) {
	setHome(t)

	args, cleanup := BuildRunArgs("claude", false)
	defer cleanup()
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "/workspace") {
		t.Errorf("BuildRunArgs missing /workspace mount in %q", joined)
	}
	if !strings.Contains(joined, "--workdir /workspace") {
		t.Errorf("BuildRunArgs missing --workdir /workspace in %q", joined)
	}
}

func TestBuildRunArgs_certInjectedWhenExists(t *testing.T) {
	home := setHome(t)
	hiveDir := filepath.Join(home, ".hive")
	if err := os.MkdirAll(hiveDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hiveDir, "extra-ca.pem"), []byte("CERT"), 0o600); err != nil {
		t.Fatal(err)
	}

	args, cleanup := BuildRunArgs("claude", false)
	defer cleanup()
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "NODE_EXTRA_CA_CERTS") {
		t.Errorf("BuildRunArgs should inject NODE_EXTRA_CA_CERTS when cert exists; args: %q", joined)
	}
	if !strings.Contains(joined, "/run/certs/extra-ca.pem") {
		t.Errorf("BuildRunArgs should bind-mount cert to /run/certs/extra-ca.pem; args: %q", joined)
	}
}

func TestBuildRunArgs_noCertInjectionWhenAbsent(t *testing.T) {
	setHome(t) // temp home, no extra-ca.pem

	args, cleanup := BuildRunArgs("claude", false)
	defer cleanup()
	for _, a := range args {
		if strings.Contains(a, "NODE_EXTRA_CA_CERTS") {
			t.Error("BuildRunArgs should not inject NODE_EXTRA_CA_CERTS when cert absent")
		}
	}
}

func TestBuildRunArgs_startsWithRunRM(t *testing.T) {
	setHome(t)

	args, cleanup := BuildRunArgs("claude", false)
	defer cleanup()
	if len(args) < 2 || args[0] != "run" || args[1] != "--rm" {
		t.Errorf("BuildRunArgs should start with 'run --rm', got %v", args[:2])
	}
}

func TestBuildRunArgs_secretEnvFileCreated(t *testing.T) {
	// When gh auth token is available, --env-file should be passed with a
	// temp file that is cleaned up by the returned func.
	// We can't guarantee gh is available in CI, so we just verify the
	// cleanup func is safe to call (no panic) even when no token exists.
	setHome(t)

	_, cleanup := BuildRunArgs("claude", false)
	// Must not panic
	cleanup()
	cleanup() // idempotent — second call also must not panic
}

// ── JoinAgents ────────────────────────────────────────────────────────────────

func TestJoinAgents(t *testing.T) {
	s := JoinAgents()
	for _, a := range agents {
		if !strings.Contains(s, a) {
			t.Errorf("JoinAgents() missing %q in %q", a, s)
		}
	}
}
