package redact

import (
	"strings"
	"testing"
)

func TestRedact_HidesSecrets(t *testing.T) {
	in := "key sk-abcdefghijklmnopqrstuvwxyz123456 and aws AKIAIOSFODNN7EXAMPLE and ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	out := Redact(in)
	for _, leak := range []string{"sk-abcdefghijklmnopqrstuvwxyz123456", "AKIAIOSFODNN7EXAMPLE", "ghp_abcdefghijklmnopqrstuvwxyz0123456789"} {
		if strings.Contains(out, leak) {
			t.Fatalf("secret leaked %q in %q", leak, out)
		}
	}
	if !strings.Contains(out, "[REDACTED") {
		t.Fatalf("expected redaction placeholder, got %q", out)
	}
}

func TestWrap_NeutralizesFenceBreakout(t *testing.T) {
	out := Wrap("hello ```world")
	if !strings.HasPrefix(out, "```") {
		t.Fatalf("expected leading fence, got %q", out)
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(out, "```\n"), "\n```")
	if strings.Contains(inner, "```") {
		t.Fatalf("inner fence not neutralized: %q", inner)
	}
	if !strings.Contains(inner, "world") {
		t.Fatalf("content lost: %q", out)
	}
}
