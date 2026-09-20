package extract

import (
	"strings"
	"testing"
)

func TestTranscript_Compacts(t *testing.T) {
	m := SessionModel{
		Source: "claude-code",
		Events: []Event{
			{Kind: "user", Text: "改登录"},
			{Kind: "assistant", Text: "先看 token.ts"},
			{Kind: "tool", ToolName: "Edit", FileEdit: &FileEdit{Path: "src/token.ts", Added: 12, Removed: 3}},
		},
	}
	tr := m.Transcript()
	for _, want := range []string{"USER: 改登录", "ASSISTANT: 先看 token.ts", "TOOL Edit src/token.ts (+12 -3)"} {
		if !strings.Contains(tr, want) {
			t.Fatalf("transcript missing %q:\n%s", want, tr)
		}
	}
}
