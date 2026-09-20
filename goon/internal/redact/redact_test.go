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

func TestRedact_PEMAndJWT(t *testing.T) {
	pem := "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIDAABODY\n-----END RSA PRIVATE KEY-----"
	if strings.Contains(Redact(pem), "MIIEowIDAABODY") {
		t.Fatal("PEM body not redacted")
	}
	jwt := "tok eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9P end"
	if strings.Contains(Redact(jwt), "eyJhbGciOiJIUzI1NiJ9") {
		t.Fatal("JWT not redacted")
	}
}

func TestWrap_FenceLongerThanContent(t *testing.T) {
	content := "a ``` b ```` c"
	out := Wrap(content)
	lines := strings.Split(out, "\n")
	fence := lines[0]
	if !strings.HasPrefix(fence, "```") {
		t.Fatalf("expected fence line, got %q", fence)
	}
	inner := strings.Join(lines[1:len(lines)-1], "\n")
	if strings.Contains(inner, fence) {
		t.Fatalf("inner contains outer fence %q: %q", fence, inner)
	}
	if !strings.Contains(inner, "a") || !strings.Contains(inner, "c") {
		t.Fatalf("content lost: %q", out)
	}
}

func TestWrap_NeutralizesSpecialTokens(t *testing.T) {
	out := Wrap("hi <|endoftext|> bye")
	if strings.Contains(out, "<|endoftext|>") {
		t.Fatalf("special token not neutralized: %q", out)
	}
	if !strings.Contains(out, "endoftext") {
		t.Fatalf("content should be preserved (only delimiters changed): %q", out)
	}
}
