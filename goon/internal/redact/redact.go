package redact

import (
	"fmt"
	"regexp"
	"strings"
)

var patterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`),          // OpenAI-style
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),             // AWS
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._-]+`), // bearer token
	regexp.MustCompile(`ghp_[A-Za-z0-9]{36}`),          // GitHub PAT
}

// Redact replaces known secret patterns with indexed placeholders.
func Redact(s string) string {
	for i, re := range patterns {
		s = re.ReplaceAllString(s, fmt.Sprintf("[REDACTED:%d]", i))
	}
	return s
}

// Wrap fences untrusted text and neutralizes embedded triple-backticks so the
// content cannot break out of the surrounding fence (prompt-injection hygiene).
// A U+200B zero-width space is inserted after the first backtick.
func Wrap(untrusted string) string {
	neutralized := strings.ReplaceAll(untrusted, "```", "`\u200b``")
	return "```\n" + neutralized + "\n```"
}
