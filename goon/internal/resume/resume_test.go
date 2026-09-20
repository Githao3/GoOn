package resume

import (
	"strings"
	"testing"

	"goon/internal/gitrepo"
	"goon/internal/handoff"
)

func TestCompose_Order(t *testing.T) {
	h := handoff.Handoff{FM: handoff.FrontMatter{ID: "x1", Git: handoff.Git{Commit: "abc"}}, Body: "## 目标\n做GoOn"}
	d := gitrepo.Report{Body: "⚠️ 3 个新提交，改了 b.txt"}
	out := Compose(h, d)
	iDrift := strings.Index(out, "Drift Report")
	iBody := strings.Index(out, "做GoOn")
	iClose := strings.Index(out, "请先消化差异")
	if !(iDrift >= 0 && iDrift < iBody && iBody < iClose) {
		t.Fatalf("order wrong: drift=%d body=%d close=%d\n%s", iDrift, iBody, iClose, out)
	}
}

func TestCompose_EmptyDriftStillPresent(t *testing.T) {
	h := handoff.Handoff{FM: handoff.FrontMatter{ID: "x1"}, Body: "## 目标\n内容"}
	out := Compose(h, gitrepo.Report{Body: ""})
	if !strings.Contains(out, "无可用对账信息") {
		t.Fatalf("empty drift should yield a placeholder, got %q", out)
	}
}
