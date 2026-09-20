package extract

import (
	"path/filepath"
	"testing"
)

func TestParseCodex(t *testing.T) {
	p, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "codex", "rollout-sample.jsonl"))
	m, err := ParseCodexFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != "codex" || m.SessionID != "c1" {
		t.Fatalf("meta wrong: %+v", m)
	}
	if m.Project.CWD != `C:\tmp` {
		t.Fatalf("cwd wrong: %+v", m.Project)
	}
	if len(m.Events) != 2 || m.Events[0].Text != "看下目录" || m.Events[1].Text != "好的" {
		t.Fatalf("events wrong: %+v", m.Events)
	}
}
