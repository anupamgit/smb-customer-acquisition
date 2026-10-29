package main

import "testing"

func TestLoadConfigLLMVars(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "openrouter")
	t.Setenv("LLM_API_KEY", "secret")
	t.Setenv("LLM_BASE_URL", "https://openrouter.ai/api/v1/")
	t.Setenv("LLM_MODEL", "openai/gpt-4o-mini")

	cfg := loadConfig()
	if !cfg.llmConfigured() {
		t.Fatal("expected LLM configured")
	}
	if cfg.llmBaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("base URL = %q", cfg.llmBaseURL)
	}
}

func TestLLMConfiguredRequiresAllFour(t *testing.T) {
	c := config{llmKey: "k", llmBaseURL: "https://example.com", llmModel: "m", llmProvider: ""}
	if c.llmConfigured() {
		t.Fatal("LLM_PROVIDER is required")
	}
}
