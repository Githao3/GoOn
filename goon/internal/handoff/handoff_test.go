package handoff

import (
	"testing"
	"time"
)

func TestRenderParseRoundTrip(t *testing.T) {
	h := Handoff{
		FM:   FrontMatter{Goon: 1, ID: "x1", Source: "claude-code", Project: "GoOn", Git: Git{Branch: "main", Commit: "abc", Dirty: true, DirtyFiles: []string{"a.go"}}},
		Body: "## 目标\n做工具\n## 当前状态\n- ✅ done\n## 关键决策\n- 用 Go\n## 改动文件\n- a.go\n## 下一步\n- 测试\n## 坑与约定\n- 无\n## 开放问题\n- 无",
	}
	text, err := Render(h)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	if got.FM.Git.Commit != "abc" || !got.FM.Git.Dirty || got.FM.Git.DirtyFiles[0] != "a.go" {
		t.Fatalf("roundtrip lost git: %+v", got.FM.Git)
	}
}

func TestValidate_Body_FlagsMissingSections(t *testing.T) {
	missing := ValidateBody("## 目标\n只有目标")
	if len(missing) == 0 || missing[0] != "当前状态" {
		t.Fatalf("expected missing sections, got %v", missing)
	}
}

func TestNewID_Format(t *testing.T) {
	ts := time.Date(2026, 9, 20, 21, 48, 3, 0, time.UTC)
	if got := NewID(ts, "claude-code"); got != "2026-09-20T21-48-03-claude-code" {
		t.Fatalf("bad id: %s", got)
	}
}

func TestParse_RejectsNoFrontmatter(t *testing.T) {
	if _, err := Parse("no front matter here"); err == nil {
		t.Fatal("expected error")
	}
}
