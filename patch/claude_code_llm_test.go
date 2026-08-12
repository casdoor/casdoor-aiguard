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
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestClaudeCodeLLMSwitchRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	configPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{
		"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "echo hi"}]}]},
		"other": "keep-me"
	}`
	if err := os.WriteFile(configPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	target := Target{AgentId: "claude-code", Path: "/usr/local/bin/claude"}

	if err := ApplyProvider(target, LLMProvider{
		BaseUrl: "https://example.com/v1",
		ApiKey:  "sk-test-123",
		Model:   "claude-x",
	}); err != nil {
		t.Fatalf("ApplyProvider: %v", err)
	}

	config := readConfig(t, configPath)
	env, ok := config["env"].(map[string]any)
	if !ok {
		t.Fatalf("env block missing after ApplyProvider: %v", config)
	}
	wantEnv := map[string]string{
		"ANTHROPIC_BASE_URL":   "https://example.com/v1",
		"ANTHROPIC_AUTH_TOKEN": "sk-test-123",
		"ANTHROPIC_MODEL":      "claude-x",
	}
	for key, want := range wantEnv {
		if got, _ := env[key].(string); got != want {
			t.Errorf("env[%q] = %q, want %q", key, got, want)
		}
	}
	if _, has := env["ANTHROPIC_SMALL_FAST_MODEL"]; has {
		t.Errorf("ANTHROPIC_SMALL_FAST_MODEL should be absent, an empty SmallFastModel was never set")
	}
	assertUnrelatedPreserved(t, config)

	if err := ClearProvider(target); err != nil {
		t.Fatalf("ClearProvider: %v", err)
	}

	config = readConfig(t, configPath)
	if _, has := config["env"]; has {
		t.Errorf("env block should be removed once empty after ClearProvider, got %v", config["env"])
	}
	assertUnrelatedPreserved(t, config)
}

// TestClaudeCodeLLMApplyClearsAStaleApiKey covers ANTHROPIC_API_KEY: Claude
// Code accepts either it or ANTHROPIC_AUTH_TOKEN, so leaving a hand-set one in
// place would leave the winning credential to Claude Code's precedence rules -
// a switch that looks applied while the old key is still in use.
func TestClaudeCodeLLMApplyClearsAStaleApiKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	configPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"env": {"ANTHROPIC_API_KEY": "sk-hand-set-official", "EDITOR": "vim"}, "other": "keep-me"}`
	if err := os.WriteFile(configPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	target := Target{AgentId: "claude-code", Path: "/usr/local/bin/claude"}
	if err := ApplyProvider(target, LLMProvider{
		BaseUrl: "https://relay.example.com/v1",
		ApiKey:  "sk-relay-key",
	}); err != nil {
		t.Fatalf("ApplyProvider: %v", err)
	}

	env, ok := readConfig(t, configPath)["env"].(map[string]any)
	if !ok {
		t.Fatalf("env block missing after ApplyProvider")
	}
	if _, has := env["ANTHROPIC_API_KEY"]; has {
		t.Errorf("ANTHROPIC_API_KEY should be cleared so it cannot outrank the token just set, got %v", env["ANTHROPIC_API_KEY"])
	}
	if got, _ := env["ANTHROPIC_AUTH_TOKEN"].(string); got != "sk-relay-key" {
		t.Errorf("ANTHROPIC_AUTH_TOKEN = %q, want %q", got, "sk-relay-key")
	}
	// An unrelated env key is not the switcher's to touch.
	if got, _ := env["EDITOR"].(string); got != "vim" {
		t.Errorf(`env["EDITOR"] = %q, want "vim"`, got)
	}
}

func TestClaudeCodeLLMClearRemovesApiKeyToo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	configPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"env": {"ANTHROPIC_API_KEY": "sk-old", "ANTHROPIC_BASE_URL": "https://relay.example.com"}}`
	if err := os.WriteFile(configPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	target := Target{AgentId: "claude-code", Path: "/usr/local/bin/claude"}
	if err := ClearProvider(target); err != nil {
		t.Fatalf("ClearProvider: %v", err)
	}

	if env, has := readConfig(t, configPath)["env"]; has {
		t.Errorf("env should be gone once every owned key is cleared, got %v", env)
	}
}

func TestClaudeCodeLLMClearIsNoopWhenUnpatched(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	target := Target{AgentId: "claude-code", Path: "/usr/local/bin/claude"}
	if err := ClearProvider(target); err != nil {
		t.Fatalf("ClearProvider on unpatched target: %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Errorf("ClearProvider should not create settings.json when there is nothing to clear, stat err = %v", err)
	}
}

// TestClaudeCodeLLMConcurrentApplyDoesNotCorruptState fires concurrent
// switches to distinct providers at one installation. stateMutex serializes
// each read-merge-write, so the file left behind must be valid JSON whose env
// keys all came from a single provider - never a mix of two.
func TestClaudeCodeLLMConcurrentApplyDoesNotCorruptState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	target := Target{AgentId: "claude-code", Path: "/usr/local/bin/claude"}

	const n = 20
	candidates := make([]LLMProvider, n)
	for i := range candidates {
		candidates[i] = LLMProvider{
			BaseUrl: fmt.Sprintf("https://provider-%d.example.com", i),
			ApiKey:  fmt.Sprintf("sk-%d", i),
			Model:   fmt.Sprintf("model-%d", i),
		}
	}

	var wg sync.WaitGroup
	errs := make(chan error, n)
	for _, provider := range candidates {
		wg.Add(1)
		go func(provider LLMProvider) {
			defer wg.Done()
			errs <- ApplyProvider(target, provider)
		}(provider)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("ApplyProvider: %v", err)
		}
	}

	configPath := filepath.Join(home, ".claude", "settings.json")
	config := readConfig(t, configPath)
	env, ok := config["env"].(map[string]any)
	if !ok {
		t.Fatalf("env block missing after concurrent ApplyProvider calls: %v", config)
	}
	baseURL, _ := env["ANTHROPIC_BASE_URL"].(string)
	apiKey, _ := env["ANTHROPIC_AUTH_TOKEN"].(string)
	model, _ := env["ANTHROPIC_MODEL"].(string)

	matched := false
	for _, candidate := range candidates {
		if candidate.BaseUrl == baseURL && candidate.ApiKey == apiKey && candidate.Model == model {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("final env {%q, %q, %q} does not match any single applied provider - looks like an interleaved write", baseURL, apiKey, model)
	}
}

func readConfig(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("cannot parse %s: %v", path, err)
	}
	return config
}

func assertUnrelatedPreserved(t *testing.T, config map[string]any) {
	t.Helper()
	if other, _ := config["other"].(string); other != "keep-me" {
		t.Errorf(`config["other"] = %q, want "keep-me"`, other)
	}
	if _, ok := config["hooks"]; !ok {
		t.Errorf("config[\"hooks\"] should survive the LLM env edit untouched")
	}
}
