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

// Package agentconfig reads and updates the live LLM API configuration of
// supported agents. API keys are deliberately represented only by HasApiKey.
package agentconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"

	"github.com/casdoor/casdoor-aiguard/agentmonitor"
	"github.com/pelletier/go-toml/v2"
)

const (
	ModeOfficial = "official"
	ModeRelay    = "relay"

	claudeBaseURL       = "ANTHROPIC_BASE_URL"
	claudeAuthToken     = "ANTHROPIC_AUTH_TOKEN"
	claudeAPIKey        = "ANTHROPIC_API_KEY"
	claudeModel         = "ANTHROPIC_MODEL"
	claudeDefaultOpus   = "ANTHROPIC_DEFAULT_OPUS_MODEL"
	claudeDefaultSonnet = "ANTHROPIC_DEFAULT_SONNET_MODEL"
	claudeDefaultHaiku  = "ANTHROPIC_DEFAULT_HAIKU_MODEL"
	codexProvider       = "aiguard"
)

var (
	writeFile       = os.WriteFile
	codexConfigLock sync.Mutex
)

type Target struct {
	AgentId string `json:"agentId"`
	Path    string `json:"path"`
	Owner   string `json:"owner"`
}

// Config is safe to return to the browser: it never contains an API key.
type Config struct {
	Mode      string `json:"mode"`
	BaseUrl   string `json:"baseUrl"`
	Model     string `json:"model"`
	HasApiKey bool   `json:"hasApiKey"`
}

type Update struct {
	AgentId string `json:"agentId"`
	Path    string `json:"path"`
	Owner   string `json:"owner"`
	Mode    string `json:"mode"`
	BaseUrl string `json:"baseUrl"`
	Model   string `json:"model"`
	ApiKey  string `json:"apiKey"`
}

func Supports(agentId string) bool {
	switch agentId {
	case "claude-code", "codex", "codex-cli":
		return true
	default:
		return false
	}
}

func Get(target Target) (Config, error) {
	switch target.AgentId {
	case "claude-code":
		home, err := ownerHome(target.Owner)
		if err != nil {
			return Config{}, err
		}
		return getClaude(filepath.Join(home, ".claude", "settings.json"))
	case "codex", "codex-cli":
		home, err := agentmonitor.ResolveCodexHome(target.Path, target.Owner)
		if err != nil {
			return Config{}, err
		}
		codexConfigLock.Lock()
		defer codexConfigLock.Unlock()
		return getCodex(home)
	default:
		return Config{}, fmt.Errorf("agent %q does not support API configuration", target.AgentId)
	}
}

func Set(update Update) (Config, error) {
	update.BaseUrl = strings.TrimSpace(update.BaseUrl)
	update.Model = strings.TrimSpace(update.Model)
	if update.Mode != ModeOfficial && update.Mode != ModeRelay {
		return Config{}, errors.New("mode must be official or relay")
	}
	if update.Mode == ModeRelay && update.BaseUrl == "" {
		return Config{}, errors.New("baseUrl is required in relay mode")
	}

	ownership, err := ownershipForOwner(update.Owner)
	if err != nil {
		return Config{}, err
	}
	switch update.AgentId {
	case "claude-code":
		home, err := ownerHome(update.Owner)
		if err != nil {
			return Config{}, err
		}
		path := filepath.Join(home, ".claude", "settings.json")
		if err := setClaude(path, update, ownership); err != nil {
			return Config{}, err
		}
		return getClaude(path)
	case "codex", "codex-cli":
		if update.Mode == ModeRelay && update.Model == "" {
			return Config{}, errors.New("model is required for Codex in relay mode")
		}
		home, err := agentmonitor.ResolveCodexHome(update.Path, update.Owner)
		if err != nil {
			return Config{}, err
		}
		codexConfigLock.Lock()
		defer codexConfigLock.Unlock()
		if err := setCodex(home, update, ownership); err != nil {
			return Config{}, err
		}
		return getCodex(home)
	default:
		return Config{}, fmt.Errorf("agent %q does not support API configuration", update.AgentId)
	}
}

func ownerHome(owner string) (string, error) {
	if owner == "" {
		return os.UserHomeDir()
	}
	account, err := user.Lookup(owner)
	if err != nil {
		return "", fmt.Errorf("cannot resolve a home directory for owner %q: %w", owner, err)
	}
	return account.HomeDir, nil
}

type fileSnapshot struct {
	path   string
	data   []byte
	mode   os.FileMode
	exists bool
}

func snapshot(path string) (fileSnapshot, error) {
	state := fileSnapshot{path: path, mode: 0o600}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return state, err
	}
	state.data, state.mode, state.exists = data, info.Mode().Perm(), true
	return state, nil
}

func writeConfigFile(state fileSnapshot, data []byte, ownership fileOwnership) error {
	directory := filepath.Dir(state.path)
	directoryMissing := false
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		directoryMissing = true
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if directoryMissing {
		if err := applyOwnership(directory, ownership); err != nil {
			return err
		}
	}
	if err := writeFile(state.path, data, state.mode); err != nil {
		return err
	}
	if state.exists {
		return nil
	}
	return applyOwnership(state.path, ownership)
}

func restore(state fileSnapshot) error {
	if !state.exists {
		err := os.Remove(state.path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.WriteFile(state.path, state.data, state.mode); err != nil {
		return err
	}
	return os.Chmod(state.path, state.mode)
}

func readJSONObject(state fileSnapshot) (map[string]any, error) {
	if !state.exists {
		return map[string]any{}, nil
	}
	value := map[string]any{}
	if err := json.Unmarshal(state.data, &value); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", state.path, err)
	}
	if value == nil {
		return nil, fmt.Errorf("cannot parse %s: root must be a JSON object", state.path)
	}
	return value, nil
}

func marshalJSON(value map[string]any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func readTOML(state fileSnapshot) (map[string]any, error) {
	value := map[string]any{}
	if !state.exists || len(strings.TrimSpace(string(state.data))) == 0 {
		return value, nil
	}
	if err := toml.Unmarshal(state.data, &value); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", state.path, err)
	}
	return value, nil
}

func getClaude(path string) (Config, error) {
	state, err := snapshot(path)
	if err != nil {
		return Config{}, err
	}
	settings, err := readJSONObject(state)
	if err != nil {
		return Config{}, err
	}
	result := Config{Mode: ModeOfficial}
	environment, _ := settings["env"].(map[string]any)
	result.BaseUrl, _ = environment[claudeBaseURL].(string)
	result.Model, _ = environment[claudeModel].(string)
	token, _ := environment[claudeAuthToken].(string)
	apiKey, _ := environment[claudeAPIKey].(string)
	result.HasApiKey = token != "" || apiKey != ""
	if result.BaseUrl != "" {
		result.Mode = ModeRelay
	}
	return result, nil
}

func setClaude(path string, update Update, ownership fileOwnership) error {
	state, err := snapshot(path)
	if err != nil {
		return err
	}
	settings, err := readJSONObject(state)
	if err != nil {
		return err
	}
	if update.Mode == ModeOfficial && !state.exists {
		return nil
	}

	environment, hasEnvironment := settings["env"].(map[string]any)
	if !hasEnvironment {
		if _, exists := settings["env"]; exists {
			return errors.New("Claude settings env must be a JSON object")
		}
		environment = map[string]any{}
	}

	if update.Mode == ModeRelay {
		hasKey := false
		if key, _ := environment[claudeAuthToken].(string); key != "" {
			hasKey = true
		}
		if key, _ := environment[claudeAPIKey].(string); key != "" {
			hasKey = true
		}
		if update.ApiKey == "" && !hasKey {
			return errors.New("apiKey is required when relay mode has no existing key")
		}
		environment[claudeBaseURL] = update.BaseUrl
		if update.ApiKey != "" {
			environment[claudeAuthToken] = update.ApiKey
			delete(environment, claudeAPIKey)
		}
		if update.Model == "" {
			deleteClaudeModels(environment)
		} else {
			for _, key := range []string{claudeModel, claudeDefaultOpus, claudeDefaultSonnet, claudeDefaultHaiku} {
				environment[key] = update.Model
			}
		}
		settings["env"] = environment
	} else {
		for _, key := range []string{claudeBaseURL, claudeAuthToken, claudeAPIKey} {
			delete(environment, key)
		}
		deleteClaudeModels(environment)
		if len(environment) == 0 {
			delete(settings, "env")
		} else {
			settings["env"] = environment
		}
	}
	data, err := marshalJSON(settings)
	if err != nil {
		return err
	}
	return writeConfigFile(state, data, ownership)
}

func deleteClaudeModels(environment map[string]any) {
	for _, key := range []string{claudeModel, claudeDefaultOpus, claudeDefaultSonnet, claudeDefaultHaiku} {
		delete(environment, key)
	}
}

func getCodex(home string) (Config, error) {
	authState, err := snapshot(filepath.Join(home, "auth.json"))
	if err != nil {
		return Config{}, err
	}
	configState, err := snapshot(filepath.Join(home, "config.toml"))
	if err != nil {
		return Config{}, err
	}
	auth, err := readJSONObject(authState)
	if err != nil {
		return Config{}, err
	}
	settings, err := readTOML(configState)
	if err != nil {
		return Config{}, err
	}

	result := Config{Mode: ModeOfficial}
	apiKey, _ := auth["OPENAI_API_KEY"].(string)
	result.HasApiKey = apiKey != ""
	result.Model, _ = settings["model"].(string)
	activeProvider, _ := settings["model_provider"].(string)
	providers, _ := settings["model_providers"].(map[string]any)
	provider, _ := providers[activeProvider].(map[string]any)
	result.BaseUrl, _ = provider["base_url"].(string)
	if result.BaseUrl != "" {
		result.Mode = ModeRelay
	}
	return result, nil
}

func setCodex(home string, update Update, ownership fileOwnership) error {
	authState, err := snapshot(filepath.Join(home, "auth.json"))
	if err != nil {
		return err
	}
	configState, err := snapshot(filepath.Join(home, "config.toml"))
	if err != nil {
		return err
	}
	auth, err := readJSONObject(authState)
	if err != nil {
		return err
	}
	settings, err := readTOML(configState)
	if err != nil {
		return err
	}

	writeAuth, writeSettings := true, true
	if update.Mode == ModeRelay {
		key, _ := auth["OPENAI_API_KEY"].(string)
		if update.ApiKey == "" && key == "" {
			return errors.New("apiKey is required when relay mode has no existing key")
		}
		auth["auth_mode"] = "apikey"
		if update.ApiKey != "" {
			auth["OPENAI_API_KEY"] = update.ApiKey
		}

		providers, exists := settings["model_providers"].(map[string]any)
		if !exists {
			if _, present := settings["model_providers"]; present {
				return errors.New("Codex model_providers must be a TOML table")
			}
			providers = map[string]any{}
		}
		providers[codexProvider] = map[string]any{
			"name":                 "AIGuard",
			"base_url":             update.BaseUrl,
			"wire_api":             "responses",
			"requires_openai_auth": true,
		}
		settings["model_provider"] = codexProvider
		settings["model"] = update.Model
		settings["model_providers"] = providers
	} else {
		if authState.exists {
			auth["auth_mode"] = "chatgpt"
			delete(auth, "OPENAI_API_KEY")
		} else {
			writeAuth = false
		}
		if configState.exists {
			delete(settings, "model_provider")
			delete(settings, "model")
			if providers, ok := settings["model_providers"].(map[string]any); ok {
				delete(providers, codexProvider)
				if len(providers) == 0 {
					delete(settings, "model_providers")
				} else {
					settings["model_providers"] = providers
				}
			}
		} else {
			writeSettings = false
		}
	}

	var authData []byte
	if writeAuth {
		authData, err = marshalJSON(auth)
		if err != nil {
			return err
		}
	}
	var configData []byte
	if writeSettings {
		configData, err = toml.Marshal(settings)
		if err != nil {
			return err
		}
	}

	if writeAuth {
		if err := writeConfigFile(authState, authData, ownership); err != nil {
			_ = restore(authState)
			return err
		}
	}
	if writeSettings {
		if err := writeConfigFile(configState, configData, ownership); err != nil {
			configRestoreErr := restore(configState)
			authRestoreErr := error(nil)
			if writeAuth {
				authRestoreErr = restore(authState)
			}
			if configRestoreErr != nil || authRestoreErr != nil {
				return fmt.Errorf("write Codex config: %v; rollback failed: config=%v auth=%v", err, configRestoreErr, authRestoreErr)
			}
			return err
		}
	}
	return nil
}
