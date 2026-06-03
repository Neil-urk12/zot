package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// UserModelsFile is the JSON format for user-defined models.
// Place a models.json in $ZOT_HOME to add models that aren't in the
// baked-in catalog or to override catalog entries.
//
// Example:
//
//	{
//	  "providers": {
//	    "openai": {
//	      "models": [
//	        {
//	          "id": "gpt-5.5",
//	          "name": "GPT-5.5",
//	          "reasoning": true,
//	          "contextWindow": 400000,
//	          "maxTokens": 128000,
//	          "priceInput": 2.50,
//	          "priceOutput": 15.00,
//	          "priceCacheRead": 0.25
//	        }
//	      ]
//	    }
//	  }
//	}
type UserModelsFile struct {
	Providers map[string]UserProvider `json:"providers"`
}

// UserProvider groups models under a provider key.
type UserProvider struct {
	Models  []UserModel       `json:"models"`
	BaseURL string            `json:"baseUrl,omitempty"`
	API     string            `json:"api,omitempty"` // "openai" | "anthropic" | "openai-responses" | "google"
	APIKey  string            `json:"apiKey,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Compat  *UserCompatConfig `json:"compat,omitempty"`
}

// UserCompatConfig holds compatibility flags for OpenAI-compatible servers.
type UserCompatConfig struct {
	SupportsDeveloperRole   *bool `json:"supportsDeveloperRole,omitempty"`
	SupportsReasoningEffort *bool `json:"supportsReasoningEffort,omitempty"`
}

// UserModel is a single model entry in the user's models.json.
type UserModel struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Reasoning       bool     `json:"reasoning"`
	ContextWindow   int      `json:"contextWindow"`
	MaxTokens       int      `json:"maxTokens"`
	PriceInput      float64  `json:"priceInput"`
	PriceOutput     float64  `json:"priceOutput"`
	PriceCacheRead  float64  `json:"priceCacheRead"`
	PriceCacheWrite float64  `json:"priceCacheWrite"`
	BaseURL         string   `json:"baseUrl,omitempty"`
	Input           []string `json:"input"` // informational only
	API             string            `json:"api"`
	APIKey          string            `json:"apiKey,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Compat          *UserCompatConfig `json:"compat,omitempty"`
}

// LoadUserModels reads a models.json file and returns the models
// converted to the internal Model type. Returns nil on any error
// (missing file, bad JSON, etc.) so the caller can treat it as
// optional without error handling.
func LoadUserModels(path string) []Model {
	models, _, _ := LoadUserModelsWithWarnings(path)
	return models
}

// LoadUserModelsWithWarnings is like LoadUserModels but also returns
// human-readable warnings about every recoverable issue it found in
// the file (unknown provider id, empty model id, malformed JSON for a
// single provider block, etc.). The caller is responsible for
// surfacing the warnings; the file is never rejected wholesale unless
// the top-level JSON itself fails to parse.
func LoadUserModelsWithWarnings(path string) ([]Model, map[string]string, []string) {
	var warnings []string
	var out []Model
	apiKeys := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil
	}
	var file UserModelsFile
	if err := json.Unmarshal(data, &file); err != nil {
		warnings = append(warnings, fmt.Sprintf("models.json: parse error: %v (file ignored)", err))
		return nil, nil, warnings
	}

	for providerName, prov := range file.Providers {
		if providerName == "" {
			warnings = append(warnings, "models.json: empty provider key skipped")
			continue
		}
		// Normalize legacy transport aliases to their provider names.
		normalized := providerName
		switch providerName {
		case "openai-responses":
			normalized = "openai"
		case "anthropic-messages":
			normalized = "anthropic"
		case "moonshot", "moonshot-ai", "kimi-code":
			normalized = "kimi"
		case "deepseek-chat", "deepseek-ai":
			normalized = "deepseek"
		}

		// Resolve provider-level apiKey with env var interpolation.
		providerAPIKey := resolveEnvVars(prov.APIKey)
		providerHeaders := resolveHeaders(prov.Headers)
		if providerAPIKey != "" {
			apiKeys[normalized] = providerAPIKey
		}

		for i, um := range prov.Models {
			if um.ID == "" {
				warnings = append(warnings, fmt.Sprintf("models.json: provider %q entry #%d has empty id; skipped", providerName, i))
				continue
			}
			// Provider-level defaults: model-level overrides.
			if um.API == "" {
				um.API = prov.API
			}
			if um.BaseURL == "" {
				um.BaseURL = prov.BaseURL
			}
			if um.APIKey == "" {
				um.APIKey = providerAPIKey
			} else {
				um.APIKey = resolveEnvVars(um.APIKey)
			}
			um.Headers = mergeHeaders(providerHeaders, resolveHeaders(um.Headers))
			if um.Compat == nil {
				um.Compat = prov.Compat
			}
			if um.ContextWindow < 0 || um.MaxTokens < 0 {
				warnings = append(warnings, fmt.Sprintf("models.json: %s/%s has negative contextWindow/maxTokens; clamped to 0", normalized, um.ID))
				if um.ContextWindow < 0 {
					um.ContextWindow = 0
				}
				if um.MaxTokens < 0 {
					um.MaxTokens = 0
				}
			}
			m := Model{
				Provider:        normalized,
				ID:              um.ID,
				DisplayName:     um.Name,
				ContextWindow:   um.ContextWindow,
				MaxOutput:       um.MaxTokens,
				Reasoning:       um.Reasoning,
				PriceInput:      um.PriceInput,
				PriceOutput:     um.PriceOutput,
				PriceCacheRead:  um.PriceCacheRead,
				PriceCacheWrite: um.PriceCacheWrite,
				BaseURL:         um.BaseURL,
				API:             um.API,
				APIKey:          um.APIKey,
				Headers:         um.Headers,
				Source:          "user",
			}
			// Compat flags: nil means "use default" (most compatible).
			if um.Compat != nil {
				m.SupportsDeveloperRole = um.Compat.SupportsDeveloperRole
				m.SupportsReasoningEffort = um.Compat.SupportsReasoningEffort
			}
			if m.DisplayName == "" {
				m.DisplayName = m.ID
			}
			if (m.API == "openai-responses" || m.API == "google") && len(m.Headers) > 0 {
				warnings = append(warnings, fmt.Sprintf("models.json: %s/%s: custom headers are not passed through for api %q", normalized, m.ID, m.API))
			}
			out = append(out, m)
		}
	}
	return out, apiKeys, warnings
}

// SetUserModels merges user-defined models into the active catalog.
// User models take precedence over both the baked-in catalog and
// live-discovered models.
func SetUserModels(models []Model) {
	if len(models) == 0 {
		return
	}
	activeMu.Lock()
	defer activeMu.Unlock()
	activeSet = true

	// Build index of current active models.
	byKey := func(p, id string) string { return p + "/" + id }
	index := make(map[string]int, len(active))
	for i, m := range active {
		index[byKey(m.Provider, m.ID)] = i
	}

	for _, um := range models {
		k := byKey(um.Provider, um.ID)
		if idx, ok := index[k]; ok {
			// Override existing entry but keep prices from user if
			// they provided them, otherwise keep catalog prices.
			existing := active[idx]
			if um.PriceInput > 0 {
				existing.PriceInput = um.PriceInput
			}
			if um.PriceOutput > 0 {
				existing.PriceOutput = um.PriceOutput
			}
			if um.PriceCacheRead > 0 {
				existing.PriceCacheRead = um.PriceCacheRead
			}
			if um.PriceCacheWrite > 0 {
				existing.PriceCacheWrite = um.PriceCacheWrite
			}
			if um.DisplayName != "" {
				existing.DisplayName = um.DisplayName
			}
			if um.ContextWindow > 0 {
				existing.ContextWindow = um.ContextWindow
			}
			if um.MaxOutput > 0 {
				existing.MaxOutput = um.MaxOutput
			}
			existing.Reasoning = um.Reasoning
			if um.BaseURL != "" {
				existing.BaseURL = um.BaseURL
			}
			if um.API != "" {
				existing.API = um.API
			}
			if len(um.Headers) > 0 {
				existing.Headers = um.Headers
			}
			if um.SupportsDeveloperRole != nil {
				existing.SupportsDeveloperRole = um.SupportsDeveloperRole
			}
			if um.SupportsReasoningEffort != nil {
				existing.SupportsReasoningEffort = um.SupportsReasoningEffort
			}
			existing.Source = "user"
			existing.Speculative = false
			active[idx] = existing
		} else {
			// New model not in catalog.
			active = append(active, um)
		}
	}
}

// userAPIKeys stores API keys resolved from models.json for user-defined
// providers. Keys are populated at load time with env var interpolation.
var (
	userAPIKeysMu sync.RWMutex
	userAPIKeys   = map[string]string{}
)

// SetUserAPIKeys stores resolved API keys from models.json.
func SetUserAPIKeys(keys map[string]string) {
	userAPIKeysMu.Lock()
	defer userAPIKeysMu.Unlock()
	userAPIKeys = keys
}

// UserAPIKey returns the models.json apiKey for a provider, or "".
func UserAPIKey(provider string) string {
	userAPIKeysMu.RLock()
	defer userAPIKeysMu.RUnlock()
	return userAPIKeys[provider]
}

// resolveEnvVars performs $VAR and ${VAR} interpolation in a string.
// Bare strings without $ prefixes are returned unchanged.
func resolveEnvVars(s string) string {
	if s == "" || !strings.Contains(s, "$") {
		return s
	}
	return os.ExpandEnv(s)
}

// resolveHeaders interpolates $VAR references in header values.
func resolveHeaders(h map[string]string) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = resolveEnvVars(v)
	}
	return out
}

// mergeHeaders returns base overlaid with override. Nil maps are treated
// as empty. Override wins on key conflict.
func mergeHeaders(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}
