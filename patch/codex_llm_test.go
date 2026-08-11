// Copyright 2025 The Casdoor Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package patch

import (
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// codexTestTarget points a Target at a scratch $CODEX_HOME owned by the
// current process user, so agentmonitor.ResolveCodexHome's "current user"
// branch honours the CODEX_HOME override this test sets.
func codexTestTarget(t *testing.T) Target {
	t.Helper()
	current, err := user.Current()
	if err != nil {
		t.Skip("cannot resolve current user:", err)
	}
	return Target{AgentId: "codex", Path: "/usr/local/bin/codex", Owner: current.Username}
}

func TestCodexLLMSwitchRoundTrip(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	target := codexTestTarget(t)

	// Seed both files with content aiguard does not own, to prove it survives.
	authPath := filepath.Join(codexHome, "auth.json")
	configPath := filepath.Join(codexHome, "config.toml")
	if err := os.WriteFile(authPath, []byte(`{"tokens":{"access_token":"unrelated"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("sandbox_mode = \"workspace-write\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ApplyProvider(target, LLMProvider{
		BaseUrl: "https://api.example.com/v1",
		ApiKey:  "sk-test-123",
		Model:   "gpt-5-codex",
	}); err != nil {
		t.Fatalf("ApplyProvider: %v", err)
	}

	auth := readJSON(t, authPath)
	if auth[codexAuthModeField] != codexAuthModeApiKey {
		t.Errorf("auth_mode = %v, want %q", auth[codexAuthModeField], codexAuthModeApiKey)
	}
	if auth[codexAuthApiKeyField] != "sk-test-123" {
		t.Errorf("OPENAI_API_KEY = %v, want sk-test-123", auth[codexAuthApiKeyField])
	}
	if tokens, ok := auth["tokens"].(map[string]any); !ok || tokens["access_token"] != "unrelated" {
		t.Errorf(`auth["tokens"] should survive untouched, got %v`, auth["tokens"])
	}

	config := readTOML(t, configPath)
	if config["model_provider"] != codexProviderId {
		t.Errorf("model_provider = %v, want %q", config["model_provider"], codexProviderId)
	}
	if config["model"] != "gpt-5-codex" {
		t.Errorf("model = %v, want gpt-5-codex", config["model"])
	}
	providers, _ := config["model_providers"].(map[string]any)
	provider, _ := providers[codexProviderId].(map[string]any)
	if provider["base_url"] != "https://api.example.com/v1" {
		t.Errorf("model_providers.aiguard.base_url = %v, want https://api.example.com/v1", provider["base_url"])
	}
	if config["sandbox_mode"] != "workspace-write" {
		t.Errorf(`config["sandbox_mode"] should survive untouched, got %v`, config["sandbox_mode"])
	}

	if err := ClearProvider(target); err != nil {
		t.Fatalf("ClearProvider: %v", err)
	}

	auth = readJSON(t, authPath)
	if auth[codexAuthModeField] != codexAuthModeChatGPT {
		t.Errorf("auth_mode after clear = %v, want %q", auth[codexAuthModeField], codexAuthModeChatGPT)
	}
	if _, has := auth[codexAuthApiKeyField]; has {
		t.Errorf("OPENAI_API_KEY should be removed after clear, got %v", auth[codexAuthApiKeyField])
	}
	if tokens, ok := auth["tokens"].(map[string]any); !ok || tokens["access_token"] != "unrelated" {
		t.Errorf(`auth["tokens"] should still survive after clear, got %v`, auth["tokens"])
	}

	config = readTOML(t, configPath)
	if _, has := config["model_provider"]; has {
		t.Errorf("model_provider should be removed after clear, got %v", config["model_provider"])
	}
	if _, has := config["model_providers"]; has {
		t.Errorf("model_providers should be removed entirely once aiguard's entry is the only one, got %v", config["model_providers"])
	}
	if config["sandbox_mode"] != "workspace-write" {
		t.Errorf(`config["sandbox_mode"] should still survive after clear, got %v`, config["sandbox_mode"])
	}
}

func TestCodexLLMApplyRequiresBaseUrlAndModel(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	target := codexTestTarget(t)

	if err := ApplyProvider(target, LLMProvider{ApiKey: "sk-test"}); err == nil {
		t.Error("ApplyProvider should reject a provider with no baseUrl")
	}
	if err := ApplyProvider(target, LLMProvider{BaseUrl: "https://api.example.com", ApiKey: "sk-test"}); err == nil {
		t.Error("ApplyProvider should reject a Codex provider with no model")
	}
}

func TestCodexLLMClearIsNoopWhenUnpatched(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	target := codexTestTarget(t)

	if err := ClearProvider(target); err != nil {
		t.Fatalf("ClearProvider on unpatched target: %v", err)
	}
	if _, err := os.Stat(filepath.Join(codexHome, "auth.json")); !os.IsNotExist(err) {
		t.Errorf("ClearProvider should not create auth.json when there is nothing to clear, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(codexHome, "config.toml")); !os.IsNotExist(err) {
		t.Errorf("ClearProvider should not create config.toml when there is nothing to clear, stat err = %v", err)
	}
}

// TestCodexLLMClearLeavesForeignModelProviderAlone proves config.toml's
// ownership check: a provider the operator configured by hand under another
// name is not aiguard's to remove.
func TestCodexLLMClearLeavesForeignModelProviderAlone(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	target := codexTestTarget(t)

	configPath := filepath.Join(codexHome, "config.toml")
	if err := os.WriteFile(configPath, []byte("model_provider = \"my-own-provider\"\nmodel = \"gpt-5\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ClearProvider(target); err != nil {
		t.Fatalf("ClearProvider: %v", err)
	}

	config := readTOML(t, configPath)
	if config["model_provider"] != "my-own-provider" {
		t.Errorf("a foreign model_provider should survive Clear, got %v", config["model_provider"])
	}
	if config["model"] != "gpt-5" {
		t.Errorf("model paired with a foreign model_provider should survive Clear, got %v", config["model"])
	}
}

// TestCodexLLMClearResetsApiKeyAuthEvenWhenHandSet documents the accepted
// limitation on the auth.json side: "apikey" is Codex's own generic auth mode,
// not a marker aiguard controls, so a hand-run `codex login --api-key` and a
// target aiguard patched both get reset to "chatgpt".
func TestCodexLLMClearResetsApiKeyAuthEvenWhenHandSet(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	target := codexTestTarget(t)

	authPath := filepath.Join(codexHome, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"sk-hand-set"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ClearProvider(target); err != nil {
		t.Fatalf("ClearProvider: %v", err)
	}

	auth := readJSON(t, authPath)
	if auth[codexAuthModeField] != codexAuthModeChatGPT {
		t.Errorf("auth_mode = %v, want %q", auth[codexAuthModeField], codexAuthModeChatGPT)
	}
	if _, has := auth[codexAuthApiKeyField]; has {
		t.Errorf("OPENAI_API_KEY should be removed, got %v", auth[codexAuthApiKeyField])
	}
}

func TestCodexLLMApplyRollsBackAuthOnConfigWriteFailure(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	target := codexTestTarget(t)

	authPath := filepath.Join(codexHome, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"tokens":{"access_token":"unrelated"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatal(err)
	}

	// Make config.toml a directory so the write of config.toml is guaranteed
	// to fail after auth.json has already been written.
	if err := os.MkdirAll(filepath.Join(codexHome, "config.toml"), 0o755); err != nil {
		t.Fatal(err)
	}

	err = ApplyProvider(target, LLMProvider{BaseUrl: "https://api.example.com", ApiKey: "sk-new", Model: "gpt-5-codex"})
	if err == nil {
		t.Fatal("ApplyProvider should fail when config.toml cannot be written")
	}

	after, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("auth.json should be rolled back on config.toml write failure:\nbefore=%s\nafter=%s", before, after)
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("cannot parse %s: %v", path, err)
	}
	return value
}

func readTOML(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := toml.Unmarshal(data, &value); err != nil {
		t.Fatalf("cannot parse %s: %v", path, err)
	}
	return value
}
