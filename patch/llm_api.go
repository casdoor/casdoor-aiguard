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
	"path/filepath"
	"strings"
	"sync"
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

var llmConfigMutex sync.Mutex

// LlmAPIConfig is safe to return to the browser: it never contains an API key.
type LlmAPIConfig struct {
	Mode      string `json:"mode"`
	BaseUrl   string `json:"baseUrl"`
	Model     string `json:"model"`
	HasApiKey bool   `json:"hasApiKey"`
}

type LlmAPIUpdate struct {
	Target
	Mode    string `json:"mode"`
	BaseUrl string `json:"baseUrl"`
	Model   string `json:"model"`
	ApiKey  string `json:"apiKey"`
}

type llmConfigLayout struct {
	kind       string
	state      Target
	claudePath string
	codexHome  string
}

func SupportsLlmAPI(agentId string) bool {
	switch agentId {
	case "claude-code", "codex", "codex-cli":
		return true
	default:
		return false
	}
}

func GetLlmAPI(target Target) (LlmAPIConfig, error) {
	llmConfigMutex.Lock()
	defer llmConfigMutex.Unlock()

	layout, err := llmLayoutOf(target)
	if err != nil {
		return LlmAPIConfig{}, err
	}
	return readLlmAPI(layout)
}

func SetLlmAPI(update LlmAPIUpdate) (LlmAPIConfig, error) {
	update.BaseUrl = strings.TrimSpace(update.BaseUrl)
	update.Model = strings.TrimSpace(update.Model)
	if update.Mode != ModeOfficial && update.Mode != ModeRelay {
		return LlmAPIConfig{}, errors.New("mode must be official or relay")
	}
	if update.Mode == ModeRelay && update.BaseUrl == "" {
		return LlmAPIConfig{}, errors.New("baseUrl is required in relay mode")
	}
	if update.Mode == ModeRelay && (update.AgentId == "codex" || update.AgentId == "codex-cli") && update.Model == "" {
		return LlmAPIConfig{}, errors.New("model is required for Codex in relay mode")
	}

	llmConfigMutex.Lock()
	defer llmConfigMutex.Unlock()

	layout, err := llmLayoutOf(update.Target)
	if err != nil {
		return LlmAPIConfig{}, err
	}
	if update.Mode == ModeOfficial {
		if err := Revert(layout.state); err != nil {
			return LlmAPIConfig{}, err
		}
		return readLlmAPI(layout)
	}

	ownership, err := ownershipForOwner(update.Owner)
	if err != nil {
		return LlmAPIConfig{}, fmt.Errorf("resolve profile owner %q: %w", update.Owner, err)
	}
	apply := func(changes *ChangeSet) error {
		switch layout.kind {
		case "claude":
			return applyClaudeRelay(changes, layout.claudePath, update, ownership)
		case "codex":
			return applyCodexRelay(changes, layout.codexHome, update, ownership)
		default:
			return errors.New("unknown LLM API configuration layout")
		}
	}
	if IsApplied(layout.state) {
		err = edit(layout.state, apply)
	} else {
		err = Apply(layout.state, apply)
	}
	if err != nil {
		return LlmAPIConfig{}, err
	}
	return readLlmAPI(layout)
}

func llmLayoutOf(target Target) (llmConfigLayout, error) {
	stateOwner := target.Owner
	if account, err := ownerAccount(target.Owner); err == nil && account.Username != "" {
		stateOwner = account.Username
	}
	switch target.AgentId {
	case "claude-code":
		home, err := homeOf(target)
		if err != nil {
			return llmConfigLayout{}, err
		}
		path := filepath.Join(home, ".claude", "settings.json")
		return llmConfigLayout{
			kind:       "claude",
			claudePath: path,
			state:      Target{AgentId: "llm-api-claude-code", Path: path, Owner: stateOwner},
		}, nil
	case "codex", "codex-cli":
		home, err := codexHomeOf(target)
		if err != nil {
			return llmConfigLayout{}, err
		}
		return llmConfigLayout{
			kind:      "codex",
			codexHome: home,
			state:     Target{AgentId: "llm-api-codex", Path: home, Owner: stateOwner},
		}, nil
	default:
		return llmConfigLayout{}, fmt.Errorf("agent %q does not support API configuration", target.AgentId)
	}
}

func readLlmAPI(layout llmConfigLayout) (LlmAPIConfig, error) {
	managed := IsApplied(layout.state)
	if layout.kind == "claude" {
		return readClaudeLlmAPI(layout.claudePath, managed)
	}
	return readCodexLlmAPI(layout.codexHome, managed)
}

func readClaudeLlmAPI(path string, managed bool) (LlmAPIConfig, error) {
	settings, _, _, err := readJSONConfigFile(path)
	if err != nil {
		return LlmAPIConfig{}, err
	}
	result := LlmAPIConfig{Mode: ModeOfficial}
	if managed {
		result.Mode = ModeRelay
	}
	environment, _ := settings["env"].(map[string]any)
	result.BaseUrl, _ = environment[claudeBaseURL].(string)
	result.Model, _ = environment[claudeModel].(string)
	token, _ := environment[claudeAuthToken].(string)
	apiKey, _ := environment[claudeAPIKey].(string)
	result.HasApiKey = token != "" || apiKey != ""
	return result, nil
}

func applyClaudeRelay(changes *ChangeSet, path string, update LlmAPIUpdate, ownership fileOwnership) error {
	settings, err := readJSONObject(changes, path)
	if err != nil {
		return err
	}
	environment, hasEnvironment := settings["env"].(map[string]any)
	if !hasEnvironment {
		if _, exists := settings["env"]; exists {
			return errors.New("Claude settings env must be a JSON object")
		}
		environment = map[string]any{}
	}

	token, _ := environment[claudeAuthToken].(string)
	apiKey, _ := environment[claudeAPIKey].(string)
	if update.ApiKey == "" && token == "" && apiKey == "" {
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
	data, err := marshalJSONObject(settings)
	if err != nil {
		return err
	}

	directory := filepath.Dir(path)
	if err := changes.MkdirAll(directory); err != nil {
		return err
	}
	if err := changes.chmodCreated(directory, 0o700); err != nil {
		return err
	}
	if err := changes.chownCreated(directory, ownership); err != nil {
		return err
	}
	if err := changes.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return changes.chownCreated(path, ownership)
}

func deleteClaudeModels(environment map[string]any) {
	for _, key := range []string{claudeModel, claudeDefaultOpus, claudeDefaultSonnet, claudeDefaultHaiku} {
		delete(environment, key)
	}
}

func readCodexLlmAPI(home string, managed bool) (LlmAPIConfig, error) {
	auth, _, _, err := readJSONConfigFile(filepath.Join(home, "auth.json"))
	if err != nil {
		return LlmAPIConfig{}, err
	}
	settings, err := readTOMLFile(filepath.Join(home, "config.toml"))
	if err != nil {
		return LlmAPIConfig{}, err
	}

	result := LlmAPIConfig{Mode: ModeOfficial}
	if managed {
		result.Mode = ModeRelay
	}
	apiKey, _ := auth["OPENAI_API_KEY"].(string)
	result.HasApiKey = apiKey != ""
	result.Model, _ = settings["model"].(string)
	activeProvider, _ := settings["model_provider"].(string)
	providers, _ := settings["model_providers"].(map[string]any)
	provider, _ := providers[activeProvider].(map[string]any)
	result.BaseUrl, _ = provider["base_url"].(string)
	return result, nil
}

func applyCodexRelay(changes *ChangeSet, home string, update LlmAPIUpdate, ownership fileOwnership) error {
	authPath := filepath.Join(home, "auth.json")
	configPath := filepath.Join(home, "config.toml")
	auth, err := readJSONObject(changes, authPath)
	if err != nil {
		return err
	}
	configData, err := changes.ReadFile(configPath)
	if err != nil {
		return err
	}
	key, _ := auth["OPENAI_API_KEY"].(string)
	if update.ApiKey == "" && key == "" {
		return errors.New("apiKey is required when relay mode has no existing key")
	}
	auth["auth_mode"] = "apikey"
	if update.ApiKey != "" {
		auth["OPENAI_API_KEY"] = update.ApiKey
	}
	authData, err := marshalJSONObject(auth)
	if err != nil {
		return err
	}
	configData, err = updateCodexConfigTOML(configData, update.Model, update.BaseUrl)
	if err != nil {
		return fmt.Errorf("cannot update %s: %w", configPath, err)
	}

	if err := changes.MkdirAll(home); err != nil {
		return err
	}
	if err := changes.chmodCreated(home, 0o700); err != nil {
		return err
	}
	if err := changes.chownCreated(home, ownership); err != nil {
		return err
	}
	if err := changes.WriteFile(authPath, authData, 0o600); err != nil {
		return err
	}
	if err := changes.chownCreated(authPath, ownership); err != nil {
		return err
	}
	if err := changes.WriteFile(configPath, configData, 0o600); err != nil {
		return err
	}
	return changes.chownCreated(configPath, ownership)
}
