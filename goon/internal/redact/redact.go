package redact

import (
	"fmt"
	"regexp"
	"strings"
)

var patterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`),                                                     // OpenAI/Anthropic (sk-, sk-proj-, sk-ant-)
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),                                                          // AWS access key id
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]+`),                                             // bearer token
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`),                                                // GitHub classic variants
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),                                              // GitHub fine-grained
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),                                              // Slack
	regexp.MustCompile(`eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`),          // JWT
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`), // PEM key
}

// Redact replaces known secret patterns with indexed placeholders.
func Redact(s string) string {
	for i, re := range patterns {
		s = re.ReplaceAllString(s, fmt.Sprintf("[REDACTED:%d]", i))
	}
	return s
}

var (
	backtickRun = regexp.MustCompile("`+")
	specialTok  = regexp.MustCompile(`<\|[^|]*\|>`)
)

// Wrap fences untrusted text so it cannot break out: the fence is strictly longer
// than the longest backtick run inside, and model special-token delimiters are
// neutralized with a visible substitution. Uses no invisible characters.
func Wrap(untrusted string) string {
	s := specialTok.ReplaceAllStringFunc(untrusted, func(m string) string {
		return strings.ReplaceAll(m, "|", "¦")
	})
	longest := 0
	for _, run := range backtickRun.FindAllString(s, -1) {
		if len(run) > longest {
			longest = len(run)
		}
	}
	n := longest + 1
	if n < 3 {
		n = 3
	}
	fence := strings.Repeat("`", n)
	return fence + "\n" + s + "\n" + fence
}
