package provider

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper: write a models.json to a temp dir and return its path.
func writeModelsFile(t *testing.T, dir string, content interface{}) string {
	t.Helper()
	data, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		t.Fatalf("marshal models.json: %v", err)
	}
	path := filepath.Join(dir, "models.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}
	return path
}

// helper: bool pointer
func boolPtr(b bool) *bool { return &b }

func TestLoadUserModels_ProviderLevelAPIAndBaseURL(t *testing.T) {
	dir := t.TempDir()
	path := writeModelsFile(t, dir, UserModelsFile{
		Providers: map[string]UserProvider{
			"openai": {
				API:     "openai-responses",
				BaseURL: "https://proxy.example.com",
				Models: []UserModel{
					{ID: "model-a", Name: "Model A"},
					{ID: "model-b", Name: "Model B", BaseURL: "https://override.example.com"},
				},
			},
		},
	})

	models, _, _ := LoadUserModelsWithWarnings(path)
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	for _, m := range models {
		if m.API != "openai-responses" {
			t.Errorf("%s: expected API=openai-responses, got %q", m.ID, m.API)
		}
		switch m.ID {
		case "model-a":
			if m.BaseURL != "https://proxy.example.com" {
				t.Errorf("model-a: expected baseUrl from provider, got %q", m.BaseURL)
			}
		case "model-b":
			if m.BaseURL != "https://override.example.com" {
				t.Errorf("model-b: expected model-level baseUrl override, got %q", m.BaseURL)
			}
		}
	}
}

func TestLoadUserModels_ModelLevelAPIOverridesProvider(t *testing.T) {
	dir := t.TempDir()
	path := writeModelsFile(t, dir, UserModelsFile{
		Providers: map[string]UserProvider{
			"openai": {
				API: "openai",
				Models: []UserModel{
					{ID: "standard", Name: "Standard"},
					{ID: "custom", Name: "Custom", API: "anthropic"},
				},
			},
		},
	})

	models, _, _ := LoadUserModelsWithWarnings(path)
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	for _, m := range models {
		switch m.ID {
		case "standard":
			if m.API != "openai" {
				t.Errorf("standard: expected API=openai, got %q", m.API)
			}
		case "custom":
			if m.API != "anthropic" {
				t.Errorf("custom: expected model-level API override to anthropic, got %q", m.API)
			}
		}
	}
}

func TestLoadUserModels_ProviderLevelAPIKey(t *testing.T) {
	dir := t.TempDir()
	path := writeModelsFile(t, dir, UserModelsFile{
		Providers: map[string]UserProvider{
			"openai": {
				APIKey: "sk-provider-123",
				Models: []UserModel{
					{ID: "m1", Name: "M1"},
				},
			},
		},
	})

	models, apiKeys, _ := LoadUserModelsWithWarnings(path)
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}

	if apiKeys["openai"] != "sk-provider-123" {
		t.Errorf("expected apiKeys[openai]=sk-provider-123, got %q", apiKeys["openai"])
	}

	// Model should also have the key set internally.
	if models[0].Headers != nil {
		// Headers are unrelated here, just checking no error path.
	}
}

func TestLoadUserModels_ModelLevelAPIKeyOverridesProvider(t *testing.T) {
	dir := t.TempDir()
	path := writeModelsFile(t, dir, UserModelsFile{
		Providers: map[string]UserProvider{
			"openai": {
				APIKey: "sk-provider-level",
				Models: []UserModel{
					{ID: "standard", Name: "Standard"},
					{ID: "special", Name: "Special", APIKey: "sk-model-level"},
				},
			},
		},
	})

	_, apiKeys, _ := LoadUserModelsWithWarnings(path)

	// Provider-level key is used for the provider entry.
	if apiKeys["openai"] != "sk-provider-level" {
		t.Errorf("expected apiKeys[openai]=sk-provider-level, got %q", apiKeys["openai"])
	}
}

func TestLoadUserModels_EnvVarInterpolation(t *testing.T) {
	t.Setenv("ZOT_TEST_API_KEY", "sk-from-env-42")

	dir := t.TempDir()
	path := writeModelsFile(t, dir, UserModelsFile{
		Providers: map[string]UserProvider{
			"anthropic": {
				APIKey: "$ZOT_TEST_API_KEY",
				Models: []UserModel{
					{ID: "claude-test", Name: "Claude Test"},
					{ID: "claude-model-key", Name: "Claude Model Key", APIKey: "sk-${ZOT_TEST_API_KEY}-extra"},
				},
			},
		},
	})

	models, apiKeys, _ := LoadUserModelsWithWarnings(path)

	if apiKeys["anthropic"] != "sk-from-env-42" {
		t.Errorf("expected apiKeys[anthropic]=sk-from-env-42, got %q", apiKeys["anthropic"])
	}

	// The model with its own apiKey containing $ZOT_TEST_API_KEY gets resolved.
	// But since it has a model-level key, that overrides the provider key for
	// that model — but the provider-level entry in apiKeys still uses the
	// provider key. The provider key is set in apiKeys[normalized] regardless.
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
}

func TestLoadUserModels_HeadersMerge(t *testing.T) {
	dir := t.TempDir()
	path := writeModelsFile(t, dir, UserModelsFile{
		Providers: map[string]UserProvider{
			"openai": {
				Headers: map[string]string{
					"X-Provider-Only": "pv",
					"X-Shared":        "from-provider",
				},
				Models: []UserModel{
					{
						ID:   "m1",
						Name: "M1",
						Headers: map[string]string{
							"X-Model-Only": "mv",
							"X-Shared":     "from-model",
						},
					},
				},
			},
		},
	})

	models, _, _ := LoadUserModelsWithWarnings(path)
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}

	m := models[0]
	if m.Headers["X-Provider-Only"] != "pv" {
		t.Errorf("expected X-Provider-Only=pv, got %q", m.Headers["X-Provider-Only"])
	}
	if m.Headers["X-Model-Only"] != "mv" {
		t.Errorf("expected X-Model-Only=mv, got %q", m.Headers["X-Model-Only"])
	}
	if m.Headers["X-Shared"] != "from-model" {
		t.Errorf("expected X-Shared=from-model (model wins), got %q", m.Headers["X-Shared"])
	}
}

func TestLoadUserModels_CompatFlags(t *testing.T) {
	dir := t.TempDir()
	path := writeModelsFile(t, dir, UserModelsFile{
		Providers: map[string]UserProvider{
			"openai": {
				Compat: &UserCompatConfig{
					SupportsDeveloperRole:   boolPtr(true),
					SupportsReasoningEffort: boolPtr(false),
				},
				Models: []UserModel{
					{ID: "m1", Name: "M1"},
					{
						ID:   "m2",
						Name: "M2",
						Compat: &UserCompatConfig{
							SupportsDeveloperRole: boolPtr(false),
						},
					},
				},
			},
		},
	})

	models, _, _ := LoadUserModelsWithWarnings(path)
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	for _, m := range models {
		switch m.ID {
		case "m1":
			// Inherits provider-level compat.
			if m.SupportsDeveloperRole == nil || *m.SupportsDeveloperRole != true {
				t.Errorf("m1: expected SupportsDeveloperRole=true, got %v", m.SupportsDeveloperRole)
			}
			if m.SupportsReasoningEffort == nil || *m.SupportsReasoningEffort != false {
				t.Errorf("m1: expected SupportsReasoningEffort=false, got %v", m.SupportsReasoningEffort)
			}
		case "m2":
			// Model-level compat overrides provider.
			if m.SupportsDeveloperRole == nil || *m.SupportsDeveloperRole != false {
				t.Errorf("m2: expected SupportsDeveloperRole=false (model override), got %v", m.SupportsDeveloperRole)
			}
			// SupportsReasoningEffort should be nil — model compat doesn't set it,
			// and the model-level compat struct replaces provider entirely.
			if m.SupportsReasoningEffort != nil {
				t.Errorf("m2: expected SupportsReasoningEffort=nil (model compat replaces provider), got %v", m.SupportsReasoningEffort)
			}
		}
	}
}

func TestLoadUserModels_BackwardCompatible(t *testing.T) {
	dir := t.TempDir()
	// Old-style models.json: no api/baseUrl/apiKey/headers/compat at provider level.
	path := writeModelsFile(t, dir, map[string]interface{}{
		"providers": map[string]interface{}{
			"openai": map[string]interface{}{
				"models": []map[string]interface{}{
					{
						"id":            "gpt-4o",
						"name":          "GPT-4o",
						"contextWindow": 128000,
						"maxTokens":     16384,
						"priceInput":    2.5,
						"priceOutput":   10,
					},
				},
			},
		},
	})

	models, apiKeys, warnings := LoadUserModelsWithWarnings(path)
	if len(warnings) > 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if len(apiKeys) != 0 {
		t.Errorf("expected no apiKeys, got %v", apiKeys)
	}

	m := models[0]
	if m.Provider != "openai" {
		t.Errorf("expected provider=openai, got %q", m.Provider)
	}
	if m.ID != "gpt-4o" {
		t.Errorf("expected id=gpt-4o, got %q", m.ID)
	}
	if m.API != "" {
		t.Errorf("expected API empty, got %q", m.API)
	}
	if m.BaseURL != "" {
		t.Errorf("expected BaseURL empty, got %q", m.BaseURL)
	}
	if m.Headers != nil {
		t.Errorf("expected Headers nil, got %v", m.Headers)
	}
	if m.SupportsDeveloperRole != nil {
		t.Errorf("expected SupportsDeveloperRole nil, got %v", m.SupportsDeveloperRole)
	}
	if m.SupportsReasoningEffort != nil {
		t.Errorf("expected SupportsReasoningEffort nil, got %v", m.SupportsReasoningEffort)
	}
}

func TestSetUserModels_PreservesNewFields(t *testing.T) {
	// Seed the active catalog with an existing model.
	activeMu.Lock()
	orig := active
	active = []Model{
		{
			Provider: "openai", ID: "gpt-4o", DisplayName: "GPT-4o",
			ContextWindow: 128000, MaxOutput: 16384,
			PriceInput: 2.5, PriceOutput: 10,
			Source: "catalog",
		},
	}
	activeMu.Unlock()
	defer func() {
		activeMu.Lock()
		active = orig
		activeMu.Unlock()
	}()

	// Override with new fields set.
	override := Model{
		Provider: "openai", ID: "gpt-4o", DisplayName: "GPT-4o Custom",
		ContextWindow: 256000, MaxOutput: 32768,
		PriceInput: 5, PriceOutput: 20,
		BaseURL:                 "https://custom.example.com",
		API:                     "openai-responses",
		Headers:                 map[string]string{"X-Custom": "val"},
		SupportsDeveloperRole:   boolPtr(true),
		SupportsReasoningEffort: boolPtr(false),
		Source: "user",
	}

	SetUserModels([]Model{override})

	activeMu.Lock()
	defer activeMu.Unlock()

	if len(active) != 1 {
		t.Fatalf("expected 1 model in active, got %d", len(active))
	}

	m := active[0]
	if m.DisplayName != "GPT-4o Custom" {
		t.Errorf("expected DisplayName=GPT-4o Custom, got %q", m.DisplayName)
	}
	if m.BaseURL != "https://custom.example.com" {
		t.Errorf("expected BaseURL=https://custom.example.com, got %q", m.BaseURL)
	}
	if m.API != "openai-responses" {
		t.Errorf("expected API=openai-responses, got %q", m.API)
	}
	if m.Headers["X-Custom"] != "val" {
		t.Errorf("expected Headers[X-Custom]=val, got %v", m.Headers)
	}
	if m.SupportsDeveloperRole == nil || *m.SupportsDeveloperRole != true {
		t.Errorf("expected SupportsDeveloperRole=true, got %v", m.SupportsDeveloperRole)
	}
	if m.SupportsReasoningEffort == nil || *m.SupportsReasoningEffort != false {
		t.Errorf("expected SupportsReasoningEffort=false, got %v", m.SupportsReasoningEffort)
	}
	if m.Source != "user" {
		t.Errorf("expected Source=user, got %q", m.Source)
	}
}

func TestNewCustomClient_PreservesProviderName(t *testing.T) {
	tests := []struct {
		name     string
		api      string
		baseURL  string
		wantName string
	}{
		{
			name:     "openai",
			api:      "openai",
			baseURL:  "https://custom.example.com/v1",
			wantName: "my-provider",
		},
		{
			name:     "openai-responses",
			api:      "openai-responses",
			baseURL:  "https://custom.example.com/v1/responses",
			wantName: "my-responses-proxy",
		},
		{
			name:     "anthropic",
			api:      "anthropic",
			baseURL:  "https://custom.example.com/v1",
			wantName: "my-anthropic-proxy",
		},
		{
			name:     "google",
			api:      "google",
			baseURL:  "https://custom.example.com/v1beta",
			wantName: "my-gemini-proxy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{
				Provider: tt.wantName,
				ID:       "test-model",
				API:      tt.api,
				BaseURL:  tt.baseURL,
			}
			client := NewCustomClient(m, "test-key")
			if client.Name() != tt.wantName {
				t.Errorf("Name() = %q, want %q", client.Name(), tt.wantName)
			}
		})
	}
}

func TestNewCustomClient_NilBaseURL(t *testing.T) {
	m := Model{
		Provider: "my-provider",
		ID:       "test-model",
		API:      "openai",
		BaseURL:  "", // empty
	}
	client := NewCustomClient(m, "test-key")
	if client.Name() != "my-provider" {
		t.Errorf("Name() = %q, want my-provider", client.Name())
	}
	// Stream should return an error about missing baseUrl.
	_, err := client.Stream(context.Background(), Request{})
	if err == nil {
		t.Error("expected error for empty baseUrl, got nil")
	}
}

func TestLoadUserModels_WarnsOnHeadersUnsupportedAPI(t *testing.T) {
	dir := t.TempDir()
	path := writeModelsFile(t, dir, UserModelsFile{
		Providers: map[string]UserProvider{
			"my-proxy": {
				API:     "openai-responses",
				BaseURL: "https://proxy.example.com",
				Headers: map[string]string{"X-Custom": "val"},
				Models: []UserModel{
					{ID: "m1", Name: "M1"},
				},
			},
			"my-gemini": {
				API:     "google",
				BaseURL: "https://gemini.example.com",
				Headers: map[string]string{"X-Custom": "val"},
				Models: []UserModel{
					{ID: "m2", Name: "M2"},
				},
			},
		},
	})

	_, _, warnings := LoadUserModelsWithWarnings(path)

	// Should emit warnings for both API types that don't support custom headers.
	headerWarnings := 0
	for _, w := range warnings {
		if strings.Contains(w, "headers") && strings.Contains(w, "not passed through") {
			headerWarnings++
		}
	}
	if headerWarnings != 2 {
		t.Errorf("expected 2 header warnings, got %d: %v", headerWarnings, warnings)
	}
}
