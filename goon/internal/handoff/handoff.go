package handoff

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Git struct {
	Branch     string   `yaml:"branch,omitempty"`
	Commit     string   `yaml:"commit,omitempty"`
	Dirty      bool     `yaml:"dirty"`
	DirtyFiles []string `yaml:"dirty_files,omitempty"`
}

type FrontMatter struct {
	Goon       int    `yaml:"goon"`
	ID         string `yaml:"id"`
	Source     string `yaml:"source"`
	Project    string `yaml:"project"`
	Git        Git    `yaml:"git"`
	Supersedes string `yaml:"supersedes,omitempty"`
}

type Handoff struct {
	FM   FrontMatter
	Body string
}

var requiredSections = []string{"目标", "当前状态", "关键决策", "改动文件", "下一步", "坑与约定", "开放问题"}

func NewID(ts time.Time, source string) string {
	return ts.UTC().Format("2006-01-02T15-04-05") + "-" + source
}

func Render(h Handoff) (string, error) {
	y, err := yaml.Marshal(h.FM)
	if err != nil {
		return "", err
	}
	return "---\n" + string(y) + "---\n\n" + h.Body + "\n", nil
}

func Parse(text string) (Handoff, error) {
	if !strings.HasPrefix(text, "---") {
		return Handoff{}, fmt.Errorf("missing frontmatter")
	}
	rest := text[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return Handoff{}, fmt.Errorf("unterminated frontmatter")
	}
	var fm FrontMatter
	if err := yaml.Unmarshal([]byte(rest[:idx]), &fm); err != nil {
		return Handoff{}, err
	}
	body := strings.TrimPrefix(rest[idx+len("\n---"):], "\n")
	body = strings.TrimPrefix(body, "\n")
	return Handoff{FM: fm, Body: strings.TrimSpace(body)}, nil
}

// ValidateBody returns the required section titles (without "## ") that are missing.
func ValidateBody(body string) []string {
	var missing []string
	for _, s := range requiredSections {
		if !strings.Contains(body, "## "+s) {
			missing = append(missing, s)
		}
	}
	return missing
}

// BlankBody returns a template containing all required sections (for manual save).
func BlankBody() string {
	var b strings.Builder
	for _, s := range requiredSections {
		fmt.Fprintf(&b, "## %s\n- \n\n", s)
	}
	return strings.TrimSpace(b.String())
}
