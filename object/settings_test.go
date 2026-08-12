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

package object

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLLMProviderMarshalJSONNeverLeaksApiKey(t *testing.T) {
	provider := LLMProvider{Id: "p1", Name: "DeepSeek", BaseUrl: "https://api.deepseek.com", ApiKey: "sk-super-secret"}

	data, err := json.Marshal(provider)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "sk-super-secret") {
		t.Fatalf("marshaled provider leaks the API key: %s", data)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, has := decoded["apiKey"]; has {
		t.Errorf(`marshaled provider should not have an "apiKey" field at all, got %v`, decoded)
	}
	if hasKey, _ := decoded["hasApiKey"].(bool); !hasKey {
		t.Errorf("hasApiKey should be true when ApiKey is set, got %v", decoded["hasApiKey"])
	}

	empty := LLMProvider{Id: "p2", Name: "No Key Yet"}
	data, err = json.Marshal(empty)
	if err != nil {
		t.Fatal(err)
	}
	decoded = nil
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if hasKey, _ := decoded["hasApiKey"].(bool); hasKey {
		t.Errorf("hasApiKey should be false when ApiKey is empty, got %v", decoded["hasApiKey"])
	}
}

func TestLLMProviderUnmarshalJSONAcceptsApiKey(t *testing.T) {
	var provider LLMProvider
	if err := json.Unmarshal([]byte(`{"id":"p1","name":"DeepSeek","baseUrl":"https://api.deepseek.com","apiKey":"sk-new"}`), &provider); err != nil {
		t.Fatal(err)
	}
	if provider.ApiKey != "sk-new" {
		t.Errorf("ApiKey = %q, want %q", provider.ApiKey, "sk-new")
	}
}

func TestPreserveApiKeys(t *testing.T) {
	previous := LLMSettings{Providers: []LLMProvider{
		{Id: "p1", Name: "DeepSeek", ApiKey: "sk-old"},
		{Id: "p2", Name: "Kimi", ApiKey: "sk-kimi"},
	}}

	incoming := LLMSettings{Providers: []LLMProvider{
		// Edited without a new key: must keep the old one.
		{Id: "p1", Name: "DeepSeek (renamed)", ApiKey: ""},
		// Rotated with a new key: must keep the new one, not the old.
		{Id: "p2", Name: "Kimi", ApiKey: "sk-kimi-rotated"},
		// Brand new provider: nothing to merge from, keeps whatever it has.
		{Id: "p3", Name: "New Provider", ApiKey: "sk-brand-new"},
	}}

	incoming.PreserveApiKeys(previous)

	want := map[string]string{"p1": "sk-old", "p2": "sk-kimi-rotated", "p3": "sk-brand-new"}
	for _, provider := range incoming.Providers {
		if got := provider.ApiKey; got != want[provider.Id] {
			t.Errorf("provider %s: ApiKey = %q, want %q", provider.Id, got, want[provider.Id])
		}
	}
}

func TestPreserveApiKeysDropsKeyWhenProviderRemoved(t *testing.T) {
	previous := LLMSettings{Providers: []LLMProvider{{Id: "p1", ApiKey: "sk-old"}}}
	incoming := LLMSettings{Providers: []LLMProvider{}}

	incoming.PreserveApiKeys(previous)

	if len(incoming.Providers) != 0 {
		t.Fatalf("deleted provider should not reappear, got %v", incoming.Providers)
	}
}

func TestValidateProviderRemovalRejectsAnActiveProvider(t *testing.T) {
	current := LLMSettings{
		Providers: []LLMProvider{{Id: "p1", Name: "DeepSeek Relay"}},
		Active:    []LLMActiveProvider{{AgentId: "claude-code", Path: "/usr/local/bin/claude", ProviderId: "p1"}},
	}
	next := LLMSettings{Providers: []LLMProvider{}} // p1 deleted

	err := current.ValidateProviderRemoval(next)
	if err == nil {
		t.Fatal("expected an error deleting a provider that claude-code is switched to")
	}
	for _, want := range []string{"DeepSeek Relay", "claude-code"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err.Error(), want)
		}
	}
}

func TestValidateProviderRemovalAllowsDeletingAnUnusedProvider(t *testing.T) {
	current := LLMSettings{
		Providers: []LLMProvider{
			{Id: "p1", Name: "In Use"},
			{Id: "p2", Name: "Unused"},
		},
		Active: []LLMActiveProvider{{AgentId: "claude-code", Path: "/usr/local/bin/claude", ProviderId: "p1"}},
	}
	next := LLMSettings{Providers: []LLMProvider{{Id: "p1", Name: "In Use"}}} // p2 deleted, p1 kept

	if err := current.ValidateProviderRemoval(next); err != nil {
		t.Errorf("deleting an unused provider should be allowed, got error: %v", err)
	}
}

func TestValidateProviderRemovalAllowsEditingAnActiveProvider(t *testing.T) {
	current := LLMSettings{
		Providers: []LLMProvider{{Id: "p1", Name: "Old Name"}},
		Active:    []LLMActiveProvider{{AgentId: "claude-code", Path: "/usr/local/bin/claude", ProviderId: "p1"}},
	}
	next := LLMSettings{Providers: []LLMProvider{{Id: "p1", Name: "New Name"}}} // renamed, not removed

	if err := current.ValidateProviderRemoval(next); err != nil {
		t.Errorf("renaming an active provider should be allowed, got error: %v", err)
	}
}

func TestValidateProviderRemovalIgnoresInstallationsOnTheDefault(t *testing.T) {
	current := LLMSettings{
		Providers: []LLMProvider{{Id: "p1", Name: "Never Selected"}},
		Active:    []LLMActiveProvider{{AgentId: "claude-code", Path: "/usr/local/bin/claude", ProviderId: ""}},
	}
	next := LLMSettings{Providers: []LLMProvider{}}

	if err := current.ValidateProviderRemoval(next); err != nil {
		t.Errorf("an installation on the system default should not block deleting an unrelated provider, got error: %v", err)
	}
}

// TestGetSettingsReturnsAnIndependentCopy regression-tests a data race:
// GetSettings used to hand back the live currentSettings pointer, so two
// callers mutating a field in place raced on the same slice. Run under -race.
func TestGetSettingsReturnsAnIndependentCopy(t *testing.T) {
	SetSettings(&Settings{LLM: LLMSettings{Providers: []LLMProvider{{Id: "p1", Name: "Original"}}}})
	t.Cleanup(func() { SetSettings(&Settings{}) })

	a := GetSettings()
	b := GetSettings()
	if &a.LLM.Providers[0] == &b.LLM.Providers[0] {
		t.Fatal("two GetSettings() calls returned providers sharing the same backing array - mutating one would race the other")
	}

	a.LLM.Providers[0].Name = "Mutated by caller A"
	if b.LLM.Providers[0].Name != "Original" {
		t.Errorf("mutating one GetSettings() result changed another's copy: got %q", b.LLM.Providers[0].Name)
	}
}

// TestMutateSettingsSerializesConcurrentUpdates fires concurrent calls that
// each append a unique Active entry. Full serialization means every append
// survives; a lost update leaves the final count short.
func TestMutateSettingsSerializesConcurrentUpdates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("policyFile", filepath.Join(dir, "policy.yaml"))
	t.Cleanup(func() { SetSettings(&Settings{}) })

	SetSettings(&Settings{})

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := MutateSettings(func(current *Settings) (*Settings, error) {
				current.LLM.Active = append(current.LLM.Active, LLMActiveProvider{
					AgentId:    fmt.Sprintf("agent-%d", i),
					ProviderId: "p",
				})
				return current, nil
			})
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()

	final := GetSettings()
	if len(final.LLM.Active) != n {
		t.Errorf("Active has %d entries after %d concurrent MutateSettings calls, want %d - an update was lost to a race", len(final.LLM.Active), n, n)
	}
}

// TestMutateSettingsWithUndoRunsUndoWhenPersistFails covers the ordering
// problem behind /api/agents/llm-provider: the agent's own config file is
// written inside build, before settings.yaml records which provider that was.
// If the persist fails, the undo has to put the agent back - otherwise its
// live config points at a provider no Active entry mentions, which the Agents
// page cannot show or switch off.
func TestMutateSettingsWithUndoRunsUndoWhenPersistFails(t *testing.T) {
	// A settings.yaml under a directory that does not exist cannot be written.
	t.Setenv("policyFile", filepath.Join(t.TempDir(), "missing", "policy.yaml"))
	t.Cleanup(func() { SetSettings(&Settings{}) })
	SetSettings(&Settings{})

	undone := false
	_, err := MutateSettingsWithUndo(func(current *Settings) (*Settings, func() error, error) {
		current.LLM.Active = append(current.LLM.Active, LLMActiveProvider{AgentId: "claude-code", ProviderId: "p1"})
		return current, func() error {
			undone = true
			return nil
		}, nil
	})
	if err == nil {
		t.Fatal("MutateSettingsWithUndo should report a settings.yaml it cannot write")
	}
	if !undone {
		t.Error("the undo should run when persisting the settings fails")
	}
	if active := GetSettings().LLM.ActiveProviderId("claude-code", "", ""); active != "" {
		t.Errorf("a failed persist should not leave the change in memory, got %q", active)
	}
}

// TestMutateSettingsWithUndoReportsAFailedUndo: when the undo also fails the
// agent really is left ahead of settings.yaml, so the operator has to hear
// about both halves rather than just the save error.
func TestMutateSettingsWithUndoReportsAFailedUndo(t *testing.T) {
	t.Setenv("policyFile", filepath.Join(t.TempDir(), "missing", "policy.yaml"))
	t.Cleanup(func() { SetSettings(&Settings{}) })
	SetSettings(&Settings{})

	_, err := MutateSettingsWithUndo(func(current *Settings) (*Settings, func() error, error) {
		return current, func() error { return fmt.Errorf("cannot reach the agent's config file") }, nil
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"save settings", "cannot reach the agent's config file"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err.Error(), want)
		}
	}
}

// TestMutateSettingsWithUndoSkipsUndoOnSuccess keeps the undo strictly a
// failure path: a switch that persists cleanly must not be walked back.
func TestMutateSettingsWithUndoSkipsUndoOnSuccess(t *testing.T) {
	t.Setenv("policyFile", filepath.Join(t.TempDir(), "policy.yaml"))
	t.Cleanup(func() { SetSettings(&Settings{}) })
	SetSettings(&Settings{})

	undone := false
	if _, err := MutateSettingsWithUndo(func(current *Settings) (*Settings, func() error, error) {
		return current, func() error {
			undone = true
			return nil
		}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if undone {
		t.Error("the undo ran even though the settings persisted")
	}
}

// TestGetSettingsNeverReturnsNilLLMSlices pins a regression in clone(): an
// earlier version used append([]LLMProvider(nil), src...), and appending zero
// elements onto a nil slice returns nil, reintroducing the JSON "null"
// normalizeLLM exists to prevent.
func TestGetSettingsNeverReturnsNilLLMSlices(t *testing.T) {
	SetSettings(&Settings{})
	t.Cleanup(func() { SetSettings(&Settings{}) })

	settings := GetSettings()
	if settings.LLM.Providers == nil {
		t.Error("GetSettings().LLM.Providers is nil, want a non-nil empty slice (marshals to JSON null, not [])")
	}
	if settings.LLM.Active == nil {
		t.Error("GetSettings().LLM.Active is nil, want a non-nil empty slice (marshals to JSON null, not [])")
	}

	data, err := json.Marshal(settings.LLM)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "null") {
		t.Errorf("LLM settings JSON contains null, want empty arrays: %s", data)
	}
}
