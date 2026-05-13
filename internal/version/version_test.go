package version

import (
	"strings"
	"testing"
)

func TestString_format(t *testing.T) {
	Version = "v1.2.3"
	Commit = "abc1234"
	BuildDate = "2026-05-12T00:00:00Z"

	s := String()
	if !strings.Contains(s, "v1.2.3") {
		t.Errorf("String() %q missing version", s)
	}
	if !strings.Contains(s, "abc1234") {
		t.Errorf("String() %q missing commit", s)
	}
	if !strings.Contains(s, "2026-05-12T00:00:00Z") {
		t.Errorf("String() %q missing build date", s)
	}
}

func TestString_defaults(t *testing.T) {
	Version = "dev"
	Commit = "unknown"
	BuildDate = "unknown"

	s := String()
	if !strings.Contains(s, "dev") {
		t.Errorf("String() %q should contain 'dev'", s)
	}
}
