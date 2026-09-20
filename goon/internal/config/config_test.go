package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ProjectOverridesGlobal(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "global.yaml")
	proj := filepath.Join(dir, "config.yaml")
	os.WriteFile(global, []byte("drift_verbosity: off\nllm:\n  model: gpt-4o-mini\n"), 0o644)
	os.WriteFile(proj, []byte("drift_verbosity: full\n"), 0o644)

	cfg, err := Load(global, proj)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DriftVerbosity != "full" {
		t.Fatalf("project should override global, got %q", cfg.DriftVerbosity)
	}
	if cfg.LLM.Model != "gpt-4o-mini" {
		t.Fatalf("global value should survive, got %q", cfg.LLM.Model)
	}
}

func TestLoad_MissingFilesUseDefaults(t *testing.T) {
	cfg, err := Load("/no/such/global", "/no/such/proj")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SalvageKeepDays != 7 {
		t.Fatalf("default salvage_keep_days expected 7, got %d", cfg.SalvageKeepDays)
	}
	// Session roots have no config-level default; the CLI resolves them from
	// the user's home dir, so an unset config must stay empty here.
	if cfg.ClaudeProjects != "" || cfg.CodexSessions != "" || cfg.OpenCodeDB != "" || cfg.ZcodeDB != "" {
		t.Fatalf("session roots should default to empty, got %+v", cfg)
	}
}

func TestLoad_ProjectOverridesSessionRoots(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "global.yaml")
	proj := filepath.Join(dir, "config.yaml")
	os.WriteFile(global, []byte("claude_projects: /global/claude\nopencode_db: /global/oc.db\n"), 0o644)
	os.WriteFile(proj, []byte("claude_projects: /proj/claude\nzcode_db: /proj/z.db\n"), 0o644)

	cfg, err := Load(global, proj)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClaudeProjects != "/proj/claude" {
		t.Fatalf("project should override global claude_projects, got %q", cfg.ClaudeProjects)
	}
	if cfg.OpenCodeDB != "/global/oc.db" {
		t.Fatalf("global opencode_db should survive, got %q", cfg.OpenCodeDB)
	}
	if cfg.ZcodeDB != "/proj/z.db" {
		t.Fatalf("project zcode_db expected, got %q", cfg.ZcodeDB)
	}
}
