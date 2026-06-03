package agent

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patriceckhart/zot/packages/provider"
)

func TestReadAgentsContextLoadsGlobalAndAncestors(t *testing.T) {
	root := t.TempDir()
	zotHome := filepath.Join(root, "zot-home")
	project := filepath.Join(root, "repo")
	nested := filepath.Join(project, "packages", "app")
	if err := os.MkdirAll(zotHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zotHome, "AGENTS.md"), []byte("global rule"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("repo rule"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "AGENTS.md"), []byte("app rule"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := readAgentsContext(nested, zotHome)
	for _, want := range []string{"global rule", "repo rule", "app rule"} {
		if !strings.Contains(got, want) {
			t.Fatalf("readAgentsContext missing %q in:\n%s", want, got)
		}
	}
	if strings.Index(got, "global rule") > strings.Index(got, "repo rule") || strings.Index(got, "repo rule") > strings.Index(got, "app rule") {
		t.Fatalf("AGENTS.md files loaded in wrong order:\n%s", got)
	}
}

func TestReadAgentsContextMissingFilesIsEmpty(t *testing.T) {
	got := readAgentsContext(t.TempDir(), t.TempDir())
	if got != "" {
		t.Fatalf("expected no context, got %q", got)
	}
}

// TestResolveFallsBackWhenConfiguredModelIsGone reproduces the
// startup failure caught by the user's screenshot: the persisted
// config.json points at a model id that's no longer in the active
// catalogue (because they edited models.json or zot's bundled
// catalogue changed). Resolve must NOT error — strands the user
// with no way to fix it from the TUI — and should repair the config
// so the next launch is silent.
func TestResolveFallsBackWhenConfiguredModelIsGone(t *testing.T) {
	t.Setenv("ZOT_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "test-key")
	// Persist a stale model id.
	stale := "gpt-5.5-pro-not-real"
	if err := SaveConfig(Config{Provider: "openai", Model: stale}); err != nil {
		t.Fatal(err)
	}

	r, err := Resolve(Args{}, false)
	if err != nil {
		t.Fatalf("Resolve refused to launch with stale model: %v", err)
	}
	if r.Model == stale {
		t.Fatalf("Resolve kept stale model %q", r.Model)
	}
	if r.Provider != "openai" {
		t.Errorf("provider drifted: got %q; want openai", r.Provider)
	}

	// Config on disk should now hold the fallback so subsequent
	// launches don't repeat the warning.
	cfg, _ := LoadConfig()
	if cfg.Model == stale {
		t.Errorf("config.json still pins the stale model %q", cfg.Model)
	}
	if cfg.Model == "" {
		t.Errorf("config.json was emptied; expected the fallback model id")
	}
}

// TestResolveExplicitFlagStaleDoesNotRepairConfig confirms the
// repair-on-disk happens ONLY when the stale id came from the
// persisted config. If the user passed --model X explicitly and X is
// unknown, we still fall back, but we don't touch their config.
func TestResolveExplicitFlagStaleDoesNotRepairConfig(t *testing.T) {
	t.Setenv("ZOT_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "test-key")
	good := "gpt-5"
	if err := SaveConfig(Config{Provider: "openai", Model: good}); err != nil {
		t.Fatal(err)
	}

	r, err := Resolve(Args{Model: "gpt-totally-fake"}, false)
	if err != nil {
		t.Fatalf("Resolve errored on unknown --model: %v", err)
	}
	if r.Model == "gpt-totally-fake" {
		t.Errorf("Resolve kept the bogus --model value")
	}
	cfg, _ := LoadConfig()
	if cfg.Model != good {
		t.Errorf("config.json was clobbered (was %q; now %q)", good, cfg.Model)
	}
}

// TestResolveCustomProvider validates full resolution of a user-defined
// custom provider: models.json apiKey, base URL, and client construction.
func TestResolveCustomProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZOT_HOME", home)

	modelsJSON := `{
		"providers": {
			"my-vllm": {
				"api": "openai",
				"baseUrl": "http://localhost:8080/v1",
				"apiKey": "test-key",
				"models": [
					{"id": "llama-3", "name": "Llama 3", "contextWindow": 8192, "maxTokens": 4096}
				]
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(home, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	models, apiKeys, _ := provider.LoadUserModelsWithWarnings(filepath.Join(home, "models.json"))
	provider.SetUserModels(models)
	provider.SetUserAPIKeys(apiKeys)

	r, err := Resolve(Args{Provider: "my-vllm", Model: "llama-3"}, true)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if r.Provider != "my-vllm" {
		t.Errorf("provider=%q want my-vllm", r.Provider)
	}
	if r.Model != "llama-3" {
		t.Errorf("model=%q want llama-3", r.Model)
	}
	if r.BaseURL != "http://localhost:8080/v1" {
		t.Errorf("baseUrl=%q want http://localhost:8080/v1", r.BaseURL)
	}
	if r.Credential != "test-key" {
		t.Errorf("credential=%q want test-key", r.Credential)
	}

	// NewClient should produce an OpenAI-compat client.
	c := r.NewClient()
	if c.Name() != "my-vllm" {
		t.Errorf("client name=%q want my-vllm", c.Name())
	}
}

// TestResolveCustomProviderDefaultModel verifies that --model can be
// omitted for a user-defined provider; Resolve auto-picks the first model.
func TestResolveCustomProviderDefaultModel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZOT_HOME", home)

	modelsJSON := `{
		"providers": {
			"my-local": {
				"api": "anthropic",
				"baseUrl": "http://localhost:9090",
				"apiKey": "key",
				"models": [
					{"id": "custom-claude", "name": "Custom Claude"}
				]
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(home, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	models, apiKeys, _ := provider.LoadUserModelsWithWarnings(filepath.Join(home, "models.json"))
	provider.SetUserModels(models)
	provider.SetUserAPIKeys(apiKeys)

	// No --model flag: should auto-pick the first model for my-local.
	r, err := Resolve(Args{Provider: "my-local"}, true)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if r.Model != "custom-claude" {
		t.Errorf("model=%q want custom-claude (auto-picked)", r.Model)
	}
}

// TestResolveCustomProviderEnvVar tests the <PROVIDER>_API_KEY env var
// fallback when models.json defines a provider without an apiKey.
func TestResolveCustomProviderEnvVar(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZOT_HOME", home)
	t.Setenv("MY_VLLM_API_KEY", "env-key-123")

	modelsJSON := `{
		"providers": {
			"my-vllm": {
				"api": "openai",
				"baseUrl": "http://localhost:8080/v1",
				"models": [{"id": "llama-3", "name": "Llama 3"}]
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(home, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	models, apiKeys, _ := provider.LoadUserModelsWithWarnings(filepath.Join(home, "models.json"))
	provider.SetUserModels(models)
	provider.SetUserAPIKeys(apiKeys)

	r, err := Resolve(Args{Provider: "my-vllm", Model: "llama-3"}, true)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if r.Credential != "env-key-123" {
		t.Errorf("credential=%q want env-key-123", r.Credential)
	}
}

func TestLoadUserModels_ShadowsKnownProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZOT_HOME", home)

	modelsJSON := `{
		"providers": {
			"openai": {
				"api": "openai",
				"baseUrl": "http://localhost:8080/v1",
				"apiKey": "custom-key",
				"models": [
					{"id": "my-model", "name": "My Model"}
				]
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(home, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// Capture stderr warnings.
	var buf bytes.Buffer
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	LoadUserModels()

	w.Close()
	os.Stderr = old
	io.Copy(&buf, r)

	if !strings.Contains(buf.String(), "openai") || !strings.Contains(buf.String(), "shadows") {
		t.Errorf("expected shadow warning for openai, got stderr: %q", buf.String())
	}
}
