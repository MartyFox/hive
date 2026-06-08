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
	t.Setenv("HIVE_AGENT_CONFIG_MODE", "")
	t.Setenv("HIVE_GH_TOKEN_MODE", "")
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

// writeHiveYAMLConfig writes content to <home>/.hive/config.yaml.
func writeHiveYAMLConfig(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".hive")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func prependFakeGH(t *testing.T, token string) {
	t.Helper()
	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	script := "#!/bin/sh\nif [ \"$1\" = auth ] && [ \"$2\" = token ]; then\n  printf '%s\\n' \"$HIVE_FAKE_GH_TOKEN\"\n  exit 0\nfi\nexit 1\n"
	if err := os.WriteFile(ghPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HIVE_FAKE_GH_TOKEN", token)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func prependFakePodmanSecret(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "podman.log")
	podmanPath := filepath.Join(binDir, "podman")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$HIVE_FAKE_PODMAN_LOG"
if [ "$1" = secret ] && [ "$2" = create ]; then
  input="$(cat)"
  printf 'stdin:%s\n' "$input" >> "$HIVE_FAKE_PODMAN_LOG"
  exit 0
fi
if [ "$1" = secret ] && [ "$2" = rm ]; then
  exit 0
fi
exit 1
`
	if err := os.WriteFile(podmanPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HIVE_FAKE_PODMAN_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func argAfter(args []string, key string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
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

func TestRegistryName_yamlFallback(t *testing.T) {
	home := setHome(t)
	writeHiveYAMLConfig(t, home, "registry: ghcr.io/team\n")

	got := RegistryName("copilot")
	want := "ghcr.io/team/hive-copilot:latest"
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

func TestNetwork_envWinsOverYAML(t *testing.T) {
	home := setHome(t)
	writeHiveYAMLConfig(t, home, "network: yaml-net\n")
	t.Setenv("HIVE_NETWORK", "env-net")

	if got := Network(); got != "env-net" {
		t.Errorf("Network() = %q, want env-net", got)
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

	if got := hiveConfigValDefault("NO_SUCH_KEY", "", "mydefault"); got != "mydefault" {
		t.Errorf("hiveConfigValDefault: got %q, want mydefault", got)
	}
}

func TestHiveConfigValDefault_envOverridesDefault(t *testing.T) {
	setHome(t)
	t.Setenv("MY_KEY", "envval")

	if got := hiveConfigValDefault("MY_KEY", "", "default"); got != "envval" {
		t.Errorf("hiveConfigValDefault: got %q, want envval", got)
	}
}

func TestHiveConfigValDefault_usesYAMLValue(t *testing.T) {
	setHome(t)

	if got := hiveConfigValDefault("MY_KEY", "fromyaml", "default"); got != "fromyaml" {
		t.Errorf("hiveConfigValDefault: got %q, want fromyaml", got)
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

func TestTlsVerifyFlag_yamlFalse(t *testing.T) {
	home := setHome(t)
	writeHiveYAMLConfig(t, home, "tlsVerify: false\n")

	if got := tlsVerifyFlag(); got != "--tls-verify=false" {
		t.Errorf("tlsVerifyFlag() yaml false = %q, want --tls-verify=false", got)
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

func TestBeadsVersionArg_default(t *testing.T) {
	setHome(t)
	t.Setenv("HIVE_BEADS_VERSION", "")

	if got := BeadsVersionArg(); got != "HIVE_BEADS_VERSION=1.0.4" {
		t.Errorf("BeadsVersionArg() = %q, want HIVE_BEADS_VERSION=1.0.4", got)
	}
}

func TestBeadsVersionArg_envOverride(t *testing.T) {
	setHome(t)
	t.Setenv("HIVE_BEADS_VERSION", "1.2.3")

	if got := BeadsVersionArg(); got != "HIVE_BEADS_VERSION=1.2.3" {
		t.Errorf("BeadsVersionArg() = %q, want HIVE_BEADS_VERSION=1.2.3", got)
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

func buildRunArgsForTest(t *testing.T, agent string, opts RunOptions) ([]string, func()) {
	t.Helper()
	args, cleanup, err := BuildRunArgs(agent, opts)
	if err != nil {
		t.Fatalf("BuildRunArgs() error: %v", err)
	}
	return args, cleanup
}

func TestBuildRunArgs_alwaysHasSecurityFlags(t *testing.T) {
	setHome(t)
	t.Setenv("HIVE_NETWORK", "test-net")

	args, cleanup := buildRunArgsForTest(t, "claude", RunOptions{})
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

	args, cleanup := buildRunArgsForTest(t, "claude", RunOptions{Interactive: true})
	defer cleanup()
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-it") {
		t.Errorf("BuildRunArgs interactive missing -it in %q", joined)
	}
}

func TestBuildRunArgs_nonInteractiveNoIT(t *testing.T) {
	setHome(t)

	args, cleanup := buildRunArgsForTest(t, "claude", RunOptions{})
	defer cleanup()
	for _, a := range args {
		if a == "-it" {
			t.Error("BuildRunArgs non-interactive should not include -it")
		}
	}
}

func TestBuildRunArgs_workspaceMount(t *testing.T) {
	setHome(t)

	args, cleanup := buildRunArgsForTest(t, "claude", RunOptions{})
	defer cleanup()
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "/workspace") {
		t.Errorf("BuildRunArgs missing /workspace mount in %q", joined)
	}
	if !strings.Contains(joined, "--workdir /workspace") {
		t.Errorf("BuildRunArgs missing --workdir /workspace in %q", joined)
	}
}

func TestBuildRunArgs_configMountsDefaultReadOnly(t *testing.T) {
	home := setHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".agents"), 0o750); err != nil {
		t.Fatal(err)
	}

	args, cleanup := buildRunArgsForTest(t, "claude", RunOptions{})
	defer cleanup()
	joined := strings.Join(args, " ")

	for _, want := range []string{
		filepath.Join(home, ".claude") + ":/home/agent/.hive-source/claude:ro,z",
		filepath.Join(home, ".agents") + ":/home/agent/.hive-source/agents:ro,z",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("BuildRunArgs missing read-only config mount %q in %q", want, joined)
		}
	}
	for _, forbidden := range []string{
		filepath.Join(home, ".claude") + ":/home/agent/.claude:ro,z",
		filepath.Join(home, ".agents") + ":/home/agent/.agents:ro,z",
	} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("BuildRunArgs should not mount read-only config at live path %q in %q", forbidden, joined)
		}
	}
}

func TestBuildRunArgs_writableConfigFlagMountsConfigReadWrite(t *testing.T) {
	home := setHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}

	args, cleanup := buildRunArgsForTest(t, "claude", RunOptions{WritableConfig: true})
	defer cleanup()
	joined := strings.Join(args, " ")
	want := filepath.Join(home, ".claude") + ":/home/agent/.claude:rw,z"
	if !strings.Contains(joined, want) {
		t.Errorf("BuildRunArgs missing read-write config mount %q in %q", want, joined)
	}
}

func TestBuildRunArgs_writableConfigFromYAML(t *testing.T) {
	home := setHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeHiveYAMLConfig(t, home, "agentConfig:\n  mode: read-write\n")

	args, cleanup := buildRunArgsForTest(t, "claude", RunOptions{})
	defer cleanup()
	joined := strings.Join(args, " ")
	want := filepath.Join(home, ".claude") + ":/home/agent/.claude:rw,z"
	if !strings.Contains(joined, want) {
		t.Errorf("BuildRunArgs missing read-write config mount %q in %q", want, joined)
	}
}

func TestAgentConfigWritableModes(t *testing.T) {
	tests := []struct {
		mode string
		want bool
	}{
		{"", false},
		{"ro", false},
		{"read-only", false},
		{"rw", true},
		{"read-write", true},
		{"writable", true},
		{"writeable", false},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			setHome(t)
			t.Setenv("HIVE_AGENT_CONFIG_MODE", tt.mode)
			if got := AgentConfigWritable(); got != tt.want {
				t.Fatalf("AgentConfigWritable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildRunArgs_configPathFromYAML(t *testing.T) {
	home := setHome(t)
	custom := filepath.Join(home, "agent-configs", "claude")
	if err := os.MkdirAll(custom, 0o750); err != nil {
		t.Fatal(err)
	}
	writeHiveYAMLConfig(t, home, "agentConfig:\n  paths:\n    claude: "+custom+"\n")

	args, cleanup := buildRunArgsForTest(t, "claude", RunOptions{})
	defer cleanup()
	joined := strings.Join(args, " ")
	want := custom + ":/home/agent/.hive-source/claude:ro,z"
	if !strings.Contains(joined, want) {
		t.Errorf("BuildRunArgs missing YAML config mount %q in %q", want, joined)
	}
}

func TestBuildRunArgs_rejectsDangerousConfigHome(t *testing.T) {
	home := setHome(t)
	t.Setenv("CLAUDE_HOME", home)

	_, _, err := BuildRunArgs("claude", RunOptions{})
	if err == nil {
		t.Fatal("BuildRunArgs() should reject home directory as config home")
	}
	if !strings.Contains(err.Error(), "too broad") {
		t.Fatalf("BuildRunArgs() error = %v, want too broad", err)
	}
}

func TestBuildRunArgs_rejectsSensitiveConfigHome(t *testing.T) {
	for _, parts := range [][]string{{".ssh"}, {".gnupg"}, {".aws"}, {".config", "gcloud"}, {".kube"}} {
		t.Run(filepath.Join(parts...), func(t *testing.T) {
			home := setHome(t)
			path := filepath.Join(append([]string{home}, parts...)...)
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("CLAUDE_HOME", path)

			_, _, err := BuildRunArgs("claude", RunOptions{})
			if err == nil {
				t.Fatal("BuildRunArgs() should reject sensitive config home")
			}
			if !strings.Contains(err.Error(), "sensitive host config") {
				t.Fatalf("BuildRunArgs() error = %v, want sensitive host config", err)
			}
		})
	}
}

func TestBuildRunArgs_noPersistentStateMountByDefault(t *testing.T) {
	home := setHome(t)

	args, cleanup := buildRunArgsForTest(t, "claude", RunOptions{})
	defer cleanup()
	joined := strings.Join(args, " ")

	statePath := filepath.Join(home, ".hive", "state", "claude")
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("state dir should not be created by default, stat err=%v", err)
	}
	for _, forbidden := range []string{statePath, "/home/agent/.hive-state"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("BuildRunArgs should not include persistent state mount %q in %q", forbidden, joined)
		}
	}
}

func TestBuildRunArgs_copilotReadOnlyProjectsConfigWithoutRuntimeHomeRedirect(t *testing.T) {
	home := setHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".copilot"), 0o750); err != nil {
		t.Fatal(err)
	}

	args, cleanup := buildRunArgsForTest(t, "copilot", RunOptions{})
	defer cleanup()
	joined := strings.Join(args, " ")

	configMount := filepath.Join(home, ".copilot") + ":/home/agent/.hive-source/copilot:ro,z"
	if !strings.Contains(joined, configMount) {
		t.Fatalf("BuildRunArgs missing read-only copilot config mount %q in %q", configMount, joined)
	}
	if strings.Contains(joined, ":/home/agent/.copilot:ro,z") {
		t.Fatalf("BuildRunArgs should not mount read-only copilot config at live path; args=%#v", args)
	}
	for i, arg := range args {
		if arg == "-e" && i+1 < len(args) && strings.HasPrefix(args[i+1], "COPILOT_HOME=") {
			t.Fatalf("BuildRunArgs should not redirect COPILOT_HOME in read-only mode; args=%#v", args)
		}
	}
}

func TestBuildRunArgs_copilotWritableConfigKeepsRuntimeHome(t *testing.T) {
	home := setHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".copilot"), 0o750); err != nil {
		t.Fatal(err)
	}

	args, cleanup := buildRunArgsForTest(t, "copilot", RunOptions{WritableConfig: true})
	defer cleanup()
	for i, arg := range args {
		if arg == "-e" && i+1 < len(args) && strings.HasPrefix(args[i+1], "COPILOT_HOME=") {
			t.Fatalf("BuildRunArgs should not redirect COPILOT_HOME when config is writable; args=%#v", args)
		}
	}
}

func TestBuildRunArgs_extraMountFromYAML(t *testing.T) {
	home := setHome(t)
	docs := filepath.Join(home, "docs")
	if err := os.MkdirAll(docs, 0o750); err != nil {
		t.Fatal(err)
	}
	writeHiveYAMLConfig(t, home, "mounts:\n  - name: docs\n    host: "+docs+"\n    container: /mnt/docs\n    mode: read-only\n")

	args, cleanup := buildRunArgsForTest(t, "claude", RunOptions{})
	defer cleanup()
	joined := strings.Join(args, " ")
	want := docs + ":/mnt/docs:ro,z"
	if !strings.Contains(joined, want) {
		t.Errorf("BuildRunArgs missing extra mount %q in %q", want, joined)
	}
}

func TestBuildRunArgs_rejectsDangerousExtraMountHost(t *testing.T) {
	home := setHome(t)
	writeHiveYAMLConfig(t, home, "mounts:\n  - name: home\n    host: ~/\n    container: /mnt/home\n    mode: read-only\n")

	_, _, err := BuildRunArgs("claude", RunOptions{})
	if err == nil {
		t.Fatal("BuildRunArgs() should reject home extra mount")
	}
	if !strings.Contains(err.Error(), "too broad") {
		t.Fatalf("BuildRunArgs() error = %v, want too broad", err)
	}
}

func TestBuildRunArgs_rejectsSensitiveExtraMountParent(t *testing.T) {
	home := setHome(t)
	writeHiveYAMLConfig(t, home, "mounts:\n  - name: config\n    host: ~/.config\n    container: /mnt/config\n    mode: read-only\n")

	_, _, err := BuildRunArgs("claude", RunOptions{})
	if err == nil {
		t.Fatal("BuildRunArgs() should reject sensitive parent extra mount")
	}
	if !strings.Contains(err.Error(), "sensitive host config") {
		t.Fatalf("BuildRunArgs() error = %v, want sensitive host config", err)
	}
}

func TestBuildRunArgs_rejectsInvalidExtraMountContainer(t *testing.T) {
	home := setHome(t)
	docs := filepath.Join(home, "docs")
	if err := os.MkdirAll(docs, 0o750); err != nil {
		t.Fatal(err)
	}
	writeHiveYAMLConfig(t, home, "mounts:\n  - name: docs\n    host: "+docs+"\n    container: /workspace/docs\n    mode: read-only\n")

	_, _, err := BuildRunArgs("claude", RunOptions{})
	if err == nil {
		t.Fatal("BuildRunArgs() should reject non-/mnt extra mount")
	}
	if !strings.Contains(err.Error(), "under /mnt") {
		t.Fatalf("BuildRunArgs() error = %v, want under /mnt", err)
	}
}

func TestBuildRunArgs_rejectsBareMntExtraMountContainer(t *testing.T) {
	home := setHome(t)
	docs := filepath.Join(home, "docs")
	if err := os.MkdirAll(docs, 0o750); err != nil {
		t.Fatal(err)
	}
	writeHiveYAMLConfig(t, home, "mounts:\n  - name: docs\n    host: "+docs+"\n    container: /mnt\n    mode: read-only\n")

	_, _, err := BuildRunArgs("claude", RunOptions{})
	if err == nil {
		t.Fatal("BuildRunArgs() should reject bare /mnt extra mount")
	}
	if !strings.Contains(err.Error(), "under /mnt") {
		t.Fatalf("BuildRunArgs() error = %v, want under /mnt", err)
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

	args, cleanup := buildRunArgsForTest(t, "claude", RunOptions{})
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

	args, cleanup := buildRunArgsForTest(t, "claude", RunOptions{})
	defer cleanup()
	for _, a := range args {
		if strings.Contains(a, "NODE_EXTRA_CA_CERTS") {
			t.Error("BuildRunArgs should not inject NODE_EXTRA_CA_CERTS when cert absent")
		}
	}
}

func TestBuildRunArgs_startsWithRunRM(t *testing.T) {
	setHome(t)

	args, cleanup := buildRunArgsForTest(t, "claude", RunOptions{})
	defer cleanup()
	if len(args) < 2 || args[0] != "run" || args[1] != "--rm" {
		t.Errorf("BuildRunArgs should start with 'run --rm', got %v", args[:2])
	}
}

func TestBuildRunArgs_defaultDoesNotInjectGitHubToken(t *testing.T) {
	setHome(t)

	args, cleanup := buildRunArgsForTest(t, "claude", RunOptions{})
	defer cleanup()
	for _, a := range args {
		if a == "--env-file" || a == "--secret" {
			t.Error("BuildRunArgs should not inject GitHub token by default")
		}
	}
}

func TestBuildRunArgs_secretCleanupIsIdempotent(t *testing.T) {
	// We can't guarantee gh is available in CI, so verify cleanup is safe
	// even when token injection is requested but no token exists.
	setHome(t)

	_, cleanup := buildRunArgsForTest(t, "claude", RunOptions{GitHubToken: true})
	// Must not panic
	cleanup()
	cleanup() // idempotent — second call also must not panic
}

func TestBuildRunArgs_rejectsUnknownGitHubTokenMode(t *testing.T) {
	setHome(t)
	t.Setenv("HIVE_GH_TOKEN_MODE", "ambient")

	_, _, err := BuildRunArgs("claude", RunOptions{})
	if err == nil {
		t.Fatal("BuildRunArgs should reject unknown GitHub token mode")
	}
	if !strings.Contains(err.Error(), `unsupported GitHub token mode "ambient"`) {
		t.Fatalf("BuildRunArgs error = %v, want unsupported token mode", err)
	}
}

func TestBuildRunArgs_githubTokenPodmanSecretForClaude(t *testing.T) {
	setHome(t)
	prependFakeGH(t, "secret-token")
	logPath := prependFakePodmanSecret(t)

	args, cleanup := buildRunArgsForTest(t, "claude", RunOptions{GitHubToken: true})
	secret := argAfter(args, "--secret")
	if secret == "" {
		t.Fatal("BuildRunArgs should add --secret when --gh-token uses default mode")
	}
	if !strings.Contains(secret, ",type=env,target=GH_TOKEN") {
		t.Fatalf("secret arg = %q, want GH_TOKEN env target", secret)
	}
	if argAfter(args, "--env-file") != "" {
		t.Fatal("podman-secret mode should not create --env-file")
	}

	cleanup()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	if !strings.Contains(log, "secret create hive-gh-token-") {
		t.Fatalf("fake podman log missing secret create: %q", log)
	}
	if !strings.Contains(log, "stdin:secret-token") {
		t.Fatalf("fake podman log missing token on stdin: %q", log)
	}
	if !strings.Contains(log, "secret rm hive-gh-token-") {
		t.Fatalf("cleanup should remove podman secret, log: %q", log)
	}
}

func TestBuildRunArgs_githubTokenPodmanSecretForCopilot(t *testing.T) {
	setHome(t)
	prependFakeGH(t, "copilot-secret")
	logPath := prependFakePodmanSecret(t)

	args, cleanup := buildRunArgsForTest(t, "copilot", RunOptions{GitHubToken: true})
	defer cleanup()
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, ",type=env,target=GH_TOKEN") {
		t.Fatalf("args missing GH_TOKEN secret target: %q", joined)
	}
	if !strings.Contains(joined, ",type=env,target=GITHUB_PERSONAL_ACCESS_TOKEN") {
		t.Fatalf("args missing Copilot token alias secret target: %q", joined)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), "secret create hive-gh-token-"); got != 2 {
		t.Fatalf("podman secret create count = %d, want 2; log=%q", got, string(data))
	}
}

func TestBuildRunArgs_githubTokenEnvFileForClaude(t *testing.T) {
	setHome(t)
	t.Setenv("HIVE_GH_TOKEN_MODE", "env-file")
	prependFakeGH(t, "test-token")

	args, cleanup := buildRunArgsForTest(t, "claude", RunOptions{GitHubToken: true})
	envFile := argAfter(args, "--env-file")
	if envFile == "" {
		t.Fatal("BuildRunArgs should add --env-file when gh token exists")
	}

	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "GH_TOKEN=test-token\n"; got != want {
		t.Fatalf("env file = %q, want %q", got, want)
	}

	cleanup()
	if _, err := os.Stat(envFile); !os.IsNotExist(err) {
		t.Fatalf("cleanup should remove env file, stat err=%v", err)
	}
}

func TestBuildRunArgs_githubTokenEnvFileForCopilot(t *testing.T) {
	setHome(t)
	t.Setenv("HIVE_GH_TOKEN_MODE", "env-file")
	prependFakeGH(t, "copilot-token")

	args, cleanup := buildRunArgsForTest(t, "copilot", RunOptions{GitHubToken: true})
	defer cleanup()
	envFile := argAfter(args, "--env-file")
	if envFile == "" {
		t.Fatal("BuildRunArgs should add --env-file when gh token exists")
	}

	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	want := "GH_TOKEN=copilot-token\nGITHUB_PERSONAL_ACCESS_TOKEN=copilot-token\n"
	if got := string(data); got != want {
		t.Fatalf("env file = %q, want %q", got, want)
	}
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
