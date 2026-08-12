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
	"os"
	"path/filepath"
)

// claudeCodeLLMEnvKeys are the settings.json "env" keys Claude Code CLI reads
// to pick its LLM endpoint. Unlike owned hook handlers (see
// isAiguardHookHandler) a plain string value carries no marker, so aiguard
// cannot tell its own writes from an operator's: hand-set values here are
// overwritten by ApplyProvider and not restored by ClearProvider. Owning
// these keys outright is the point of the switcher.
//
// ANTHROPIC_API_KEY is owned but never written - claudeCodeLLMEnvValues has no
// entry for it, so it is cleared on both Apply and Clear. Claude Code accepts
// either it or ANTHROPIC_AUTH_TOKEN, so leaving a stale one next to the token
// a provider just set would leave the winning credential up to Claude Code's
// precedence rules, and the switch would look applied while the old key was
// still in use.
var claudeCodeLLMEnvKeys = []string{
	"ANTHROPIC_BASE_URL",
	"ANTHROPIC_AUTH_TOKEN",
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_MODEL",
	"ANTHROPIC_SMALL_FAST_MODEL",
}

type claudeCodeLLMSwitcher struct{}

func init() { registerLLMSwitcher(claudeCodeLLMSwitcher{}) }

func (claudeCodeLLMSwitcher) AgentId() string { return "claude-code" }

func (claudeCodeLLMSwitcher) Supported() bool { return true }

func (claudeCodeLLMSwitcher) ApplyProvider(target Target, provider LLMProvider) error {
	return updateClaudeCodeLLMEnv(target, claudeCodeLLMEnvValues(provider))
}

func (claudeCodeLLMSwitcher) ClearProvider(target Target) error {
	return updateClaudeCodeLLMEnv(target, nil)
}

// claudeCodeLLMEnvValues maps a saved provider onto the "env" keys
// claudeCodeLLMEnvKeys owns. Claude Desktop's Code tab reads the same file on
// Windows (see claude_desktop_llm.go) and reuses this rather than duplicating it.
func claudeCodeLLMEnvValues(provider LLMProvider) map[string]string {
	return map[string]string{
		"ANTHROPIC_BASE_URL":         provider.BaseUrl,
		"ANTHROPIC_AUTH_TOKEN":       provider.ApiKey,
		"ANTHROPIC_MODEL":            provider.Model,
		"ANTHROPIC_SMALL_FAST_MODEL": provider.SmallFastModel,
	}
}

// updateClaudeCodeLLMEnv sets or removes claudeCodeLLMEnvKeys in
// settings.json's "env" object, preserving every other key in the file
// (including a hooks block a Patcher installed). values is nil to clear every
// owned key; otherwise a "" value clears just that one, so a provider saved
// without a model override clears one a previous provider left behind rather
// than writing an empty string.
//
// Locked on stateMutex, shared with patch/state.go's ChangeSet.Apply/Revert:
// without it a switch racing a patcher's own read-modify-write of the same
// installation could discard one of the two changes.
func updateClaudeCodeLLMEnv(target Target, values map[string]string) error {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	configPath, err := claudeCodeConfigPath(target)
	if err != nil {
		return err
	}
	config, mode, exists, err := readJSONConfigFile(configPath)
	if err != nil {
		return err
	}
	if !exists && values == nil {
		return nil
	}

	env, ok := objectValue(config["env"])
	if !ok {
		env = map[string]any{}
	}

	changed := false
	for _, key := range claudeCodeLLMEnvKeys {
		value := values[key]
		if value == "" {
			if _, had := env[key]; had {
				delete(env, key)
				changed = true
			}
			continue
		}
		if current, ok := env[key].(string); !ok || current != value {
			env[key] = value
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if len(env) == 0 {
		delete(config, "env")
	} else {
		config["env"] = env
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	return writeJSONConfigFile(configPath, config, mode)
}
