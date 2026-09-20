package distill

import (
	"strings"
	"testing"

	"goon/internal/extract"
)

type fakeChat struct{ reply string }

func (f fakeChat) Complete(string) (string, error) { return f.reply, nil }

func TestBuildPrompt_IncludesSectionsAndTranscript(t *testing.T) {
	m := extract.SessionModel{Source: "claude-code", Events: []extract.Event{{Kind: "user", Text: "加登录"}}}
	p := BuildPrompt(m)
	for _, want := range []string{"USER: 加登录", "## 目标", "## 下一步", "不可信"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt missing %q:\n%s", want, p)
		}
	}
}

func TestRun_RedactsOutput(t *testing.T) {
	m := extract.SessionModel{Source: "codex"}
	body, err := Run(m, fakeChat{reply: "## 目标\nkey sk-abcdefghijklmnopqrstuvwxyz123456"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "sk-abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatalf("output not redacted: %q", body)
	}
}

func TestTruncateRunes_KeepsValidUTF8AndBounds(t *testing.T) {
	long := strings.Repeat("汉", 5000)
	out := truncateRunes(long, 100)
	if !strings.Contains(out, "…(中略)…") {
		t.Fatalf("expected middle ellipsis, got %q", out)
	}
	if strings.ContainsRune(out, 0xFFFD) {
		t.Fatalf("truncation split a rune (found replacement char)")
	}
	// short input returned unchanged
	if truncateRunes("abc", 100) != "abc" {
		t.Fatal("short input should be unchanged")
	}
}
