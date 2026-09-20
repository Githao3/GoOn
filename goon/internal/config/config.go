package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type LLM struct {
	Provider  string `yaml:"provider"`
	BaseURL   string `yaml:"base_url"`
	APIKeyEnv string `yaml:"api_key_env"`
	Model     string `yaml:"model"`
}

// Config holds user-tunable settings. The four per-client session roots are
// optional: leaving one empty means "resolve from the user's home dir", which
// the CLI does (config stays free of home-path knowledge).
type Config struct {
	HandoffDir      string `yaml:"handoff_dir"`
	SalvageDir      string `yaml:"salvage_dir"`
	SalvageKeepDays int    `yaml:"salvage_keep_days"`
	DriftVerbosity  string `yaml:"drift_verbosity"`
	LLM             LLM    `yaml:"llm"`

	ClaudeProjects string `yaml:"claude_projects"`
	CodexSessions  string `yaml:"codex_sessions"`
	OpenCodeDB     string `yaml:"opencode_db"`
	ZcodeDB        string `yaml:"zcode_db"`
}

func defaults() Config {
	return Config{
		HandoffDir:      ".goon/handoffs",
		SalvageDir:      ".goon/salvage",
		SalvageKeepDays: 7,
		DriftVerbosity:  "summary",
		LLM:             LLM{Provider: "openai-compatible", APIKeyEnv: "GOON_LLM_KEY", Model: "gpt-4o-mini"},
	}
}

// Load reads global then project YAML; later overrides earlier; missing files are skipped.
func Load(globalPath, projectPath string) (Config, error) {
	cfg := defaults()
	for _, p := range []string{globalPath, projectPath} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}
