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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/casdoor/casdoor-aiguard/agentmonitor"
	"github.com/casdoor/casdoor-aiguard/util"
	"github.com/pelletier/go-toml/v2"
)

// Codex CLI splits its LLM endpoint across two files under $CODEX_HOME:
// auth.json (OPENAI_API_KEY, auth_mode) and config.toml (model_provider,
// model, model_providers.<id>.base_url). Switching touches both.
//
// auth.json is machine-written and holds nothing but data, so it is read,
// mutated and marshaled whole. config.toml is the opposite - operators keep
// MCP servers, profiles and sandbox settings in there, with comments - so it
// is edited line by line instead (see codex_toml.go): a switch changes the
// handful of keys aiguard owns and leaves the rest of the file byte-identical.
//
// config.toml's model_provider carries a value aiguard controls ("aiguard"),
// so ClearProvider only removes those keys when model_provider is exactly
// that - one the operator configured by hand under another name is left alone.
//
// auth.json has no such marker: "apikey" is Codex's own generic auth mode, so
// a hand-run `codex login --api-key` is indistinguishable from a target
// aiguard patched, and clearing resets it either way.
const (
	codexAuthApiKeyField = "OPENAI_API_KEY"
	codexAuthModeField   = "auth_mode"
	codexAuthModeApiKey  = "apikey"
	codexAuthModeChatGPT = "chatgpt"
	codexProviderId      = "aiguard"
)

type codexLLMSwitcher struct{ id string }

func init() {
	registerLLMSwitcher(codexLLMSwitcher{id: "codex"})
	registerLLMSwitcher(codexLLMSwitcher{id: "codex-cli"})
}

func (p codexLLMSwitcher) AgentId() string { return p.id }

func (p codexLLMSwitcher) Supported() bool { return true }

func (p codexLLMSwitcher) ApplyProvider(target Target, provider LLMProvider) error {
	if provider.BaseUrl == "" {
		return errors.New("baseUrl is required to switch Codex to a relay provider")
	}
	if provider.Model == "" {
		return errors.New("Codex requires a model - edit this provider and set one before switching Codex to it")
	}
	home, err := agentmonitor.ResolveCodexHome(target.Path, target.Owner)
	if err != nil {
		return err
	}
	return updateCodexLLMConfig(home, func(auth, config map[string]any) (codexTOMLEdit, bool) {
		auth[codexAuthModeField] = codexAuthModeApiKey
		auth[codexAuthApiKeyField] = provider.ApiKey

		return codexTOMLEdit{
			Root: []codexTOMLSetting{
				{Key: "model_provider", Value: strconv.Quote(codexProviderId)},
				{Key: "model", Value: strconv.Quote(provider.Model)},
			},
			Provider: []codexTOMLSetting{
				{Key: "name", Value: strconv.Quote("AIGuard")},
				{Key: "base_url", Value: strconv.Quote(provider.BaseUrl)},
				{Key: "wire_api", Value: strconv.Quote("responses")},
				{Key: "requires_openai_auth", Value: "true"},
			},
		}, true
	})
}

func (p codexLLMSwitcher) ClearProvider(target Target) error {
	home, err := agentmonitor.ResolveCodexHome(target.Path, target.Owner)
	if err != nil {
		return err
	}
	return updateCodexLLMConfig(home, func(auth, config map[string]any) (codexTOMLEdit, bool) {
		changed := false
		if mode, _ := auth[codexAuthModeField].(string); mode == codexAuthModeApiKey {
			delete(auth, codexAuthApiKeyField)
			auth[codexAuthModeField] = codexAuthModeChatGPT
			changed = true
		}

		edit := codexTOMLEdit{}
		if activeProvider, _ := config["model_provider"].(string); activeProvider == codexProviderId {
			// An empty Value removes the key rather than setting it.
			edit.Root = []codexTOMLSetting{{Key: "model_provider"}, {Key: "model"}}
			changed = true
		}
		if providers, ok := config["model_providers"].(map[string]any); ok {
			if _, has := providers[codexProviderId]; has {
				edit.DropProviderTable = true
				changed = true
			}
		}
		return edit, changed
	})
}

// updateCodexLLMConfig applies mutate to Codex's auth.json and config.toml
// together, skipping the write when mutate reports nothing changed (so
// ClearProvider on an already-default target creates neither file).
//
// mutate edits the auth map in place and returns the config.toml edit to make.
// The new config.toml bytes are produced before anything is written, so a
// config.toml aiguard cannot read or edit fails with auth.json untouched. Past
// that point only I/O can fail, and if it does auth.json is rolled back: a
// partial write would leave Codex with a relay key but no provider pointing at
// it, or the reverse on the way back to the default.
//
// Locked on stateMutex for the reason updateClaudeCodeLLMEnv is, plus one of
// its own: codex and codex-cli often share a $CODEX_HOME.
func updateCodexLLMConfig(home string, mutate func(auth, config map[string]any) (codexTOMLEdit, bool)) error {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	authPath := filepath.Join(home, "auth.json")
	configPath := filepath.Join(home, "config.toml")

	authBackup, authExisted, err := readFileIfExists(authPath)
	if err != nil {
		return err
	}
	auth, authMode, _, err := readJSONConfigFile(authPath)
	if err != nil {
		return err
	}
	config, configData, configMode, err := readCodexTOML(configPath)
	if err != nil {
		return err
	}

	edit, changed := mutate(auth, config)
	if !changed {
		return nil
	}

	var updatedConfig []byte
	if !edit.isEmpty() {
		updatedConfig, err = editCodexTOML(configData, edit)
		if err != nil {
			return fmt.Errorf("cannot update %s: %w", configPath, err)
		}
	}

	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	if err := writeJSONConfigFile(authPath, auth, authMode); err != nil {
		return err
	}
	if updatedConfig == nil {
		return nil
	}
	if err := util.AtomicWriteFile(configPath, updatedConfig, configMode); err != nil {
		restoreFileState(authPath, authBackup, authExisted, authMode)
		return fmt.Errorf("write Codex config.toml: %w", err)
	}
	return nil
}

func readFileIfExists(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// restoreFileState undoes a write updateCodexLLMConfig cannot leave in place:
// delete a file that did not exist before, or put back the bytes of one that
// did. Errors are swallowed on purpose - this runs on the error path of a
// failed write, and the original error is what the caller needs to see.
func restoreFileState(path string, data []byte, existed bool, mode os.FileMode) {
	if !existed {
		_ = os.Remove(path)
		return
	}
	// Atomic, not a plain os.WriteFile: this restore is the last thing between
	// the operator and a half-switched Codex install, so it must not be able to
	// leave auth.json truncated itself.
	_ = util.AtomicWriteFile(path, data, mode)
}

// readCodexTOML reads config.toml twice over: as the parsed table a switcher
// makes its decisions from, and as the original bytes editCodexTOML edits in
// place. A missing file reads as an empty config with no bytes yet.
func readCodexTOML(path string) (map[string]any, []byte, os.FileMode, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil, 0o600, nil
	}
	if err != nil {
		return nil, nil, 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, 0, err
	}
	config := map[string]any{}
	if len(strings.TrimSpace(string(data))) == 0 {
		return config, data, info.Mode().Perm(), nil
	}
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, nil, 0, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return config, data, info.Mode().Perm(), nil
}
