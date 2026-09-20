package extract

import (
	"path/filepath"
	"testing"
)

func TestParseClaude(t *testing.T) {
	p, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "claude", "sample.jsonl"))
	m, err := ParseClaudeFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != "claude-code" || m.SessionID != "s1" {
		t.Fatalf("meta wrong: %+v", m)
	}
	if m.Project.CWD != `D:\proj` || m.Project.Branch != "main" {
		t.Fatalf("project wrong: %+v", m.Project)
	}
	if len(m.Events) != 3 {
		t.Fatalf("expected 3 non-sidechain events, got %d: %+v", len(m.Events), m.Events)
	}
	if m.Events[2].Kind != "tool" || m.Events[2].ToolName != "Bash" {
		t.Fatalf("event[2] should be Bash tool, got %+v", m.Events[2])
	}
}
