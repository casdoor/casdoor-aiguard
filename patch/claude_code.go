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
	"slices"
	"strings"

	"github.com/casdoor/casdoor-aiguard/agenthook"
	"github.com/casdoor/casdoor-aiguard/conf"
)

const claudeCodeHookTimeoutSeconds = 5

var claudeCodeHookEvents = []string{
	"SessionStart",
	"UserPromptSubmit",
	"PreToolUse",
	"PostToolUse",
	"PostToolUseFailure",
	"PermissionRequest",
	"PermissionDenied",
	"SubagentStart",
	"SubagentStop",
	"PreCompact",
	"PostCompact",
	"Stop",
	"StopFailure",
	"SessionEnd",
}

type claudeCodePatcher struct{}

func init() {
	register(claudeCodePatcher{})
}

func (claudeCodePatcher) AgentId() string { return "claude-code" }

func (claudeCodePatcher) Supported() bool { return true }

func (p claudeCodePatcher) Patch(target Target) error {
	configPath, err := p.configPath(target)
	if err != nil {
		return err
	}
	hookCommand, hookArguments, err := p.hookCommand(target)
	if err != nil {
		return err
	}

	config, mode, _, err := readClaudeCodeConfig(configPath)
	if err != nil {
		return err
	}
	changed, err := installClaudeCodeHooks(config, hookCommand, hookArguments)
	if err != nil {
		return fmt.Errorf("cannot merge Claude Code hooks: %w", err)
	}
	if !changed {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	return writeClaudeCodeConfig(configPath, config, mode)
}

func (p claudeCodePatcher) Unpatch(target Target) error {
	configPath, err := p.configPath(target)
	if err != nil {
		return err
	}
	config, mode, exists, err := readClaudeCodeConfig(configPath)
	if err != nil || !exists {
		return err
	}
	if !removeClaudeCodeHooks(config) {
		return nil
	}
	return writeClaudeCodeConfig(configPath, config, mode)
}

func (p claudeCodePatcher) Status(target Target) (Status, error) {
	configPath, err := p.configPath(target)
	if err != nil {
		return Status{}, err
	}
	config, _, exists, err := readClaudeCodeConfig(configPath)
	if err != nil {
		return Status{}, err
	}
	if !exists {
		return Status{Detail: "not patched"}, nil
	}

	hookState := claudeCodeHooksState(config)
	if hookState == 0 {
		return Status{Detail: "not patched"}, nil
	}
	if hookState < len(claudeCodeHookEvents) {
		return Status{Detail: fmt.Sprintf("Claude Code hooks are incomplete (%d/%d active)", hookState, len(claudeCodeHookEvents))}, nil
	}

	return Status{Patched: true, Detail: "Hooks active"}, nil
}

func (claudeCodePatcher) configPath(target Target) (string, error) {
	home, err := homeOf(target)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

func (p claudeCodePatcher) hookCommand(target Target) (string, []string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", nil, fmt.Errorf("cannot resolve the aiguard binary to register: %w", err)
	}
	return executable, []string{
		agenthook.Subcommand,
		"--agent", p.AgentId(),
		"--records-url", conf.GetRecordsIngestUrl(),
		"--agent-path", target.Path,
	}, nil
}

func readClaudeCodeConfig(path string) (map[string]any, os.FileMode, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, 0o600, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, false, err
	}
	config := map[string]any{}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, 0, false, fmt.Errorf("cannot parse %s: empty file", path)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, 0, false, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	if config == nil {
		return nil, 0, false, fmt.Errorf("cannot parse %s: root must be a JSON object", path)
	}
	return config, info.Mode().Perm(), true, nil
}

func writeClaudeCodeConfig(path string, config map[string]any, mode os.FileMode) error {
	updated, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(updated, '\n'), mode)
}

func installClaudeCodeHooks(config map[string]any, command string, arguments []string) (bool, error) {
	rawHooks, hooksExist := config["hooks"]
	hooks, ok := objectValue(rawHooks)
	if hooksExist && !ok {
		return false, fmt.Errorf("hooks must be a JSON object")
	}
	if !hooksExist {
		hooks = map[string]any{}
		config["hooks"] = hooks
	}

	changed := false
	for _, eventName := range claudeCodeHookEvents {
		if eventHasAiguardHook(hooks[eventName]) {
			continue
		}
		rawGroups, eventExists := hooks[eventName]
		groups, ok := rawGroups.([]any)
		if eventExists && !ok {
			return false, fmt.Errorf("hooks.%s must be a JSON array", eventName)
		}
		handler := map[string]any{
			"type":    "command",
			"command": command,
			"args":    arguments,
			"async":   true,
			"timeout": claudeCodeHookTimeoutSeconds,
		}
		group := map[string]any{"hooks": []any{handler}}
		if hookEventSupportsMatcher(eventName) {
			group["matcher"] = ""
		}
		hooks[eventName] = append(groups, group)
		changed = true
	}
	return changed, nil
}

func hookEventSupportsMatcher(eventName string) bool {
	switch eventName {
	case "PreToolUse", "PostToolUse", "PostToolUseFailure", "PermissionRequest", "PermissionDenied":
		return true
	default:
		return false
	}
}

func claudeCodeHooksState(config map[string]any) int {
	hooks, ok := objectValue(config["hooks"])
	if !ok {
		return 0
	}
	active := 0
	for _, eventName := range claudeCodeHookEvents {
		if eventHasAiguardHook(hooks[eventName]) {
			active++
		}
	}
	return active
}

func eventHasAiguardHook(value any) bool {
	groups, ok := value.([]any)
	if !ok {
		return false
	}
	for _, rawGroup := range groups {
		group, ok := objectValue(rawGroup)
		if !ok {
			continue
		}
		handlers, ok := group["hooks"].([]any)
		if !ok {
			continue
		}
		for _, rawHandler := range handlers {
			handler, ok := objectValue(rawHandler)
			if !ok || !isAiguardHookHandler(handler) {
				continue
			}
			asynchronous, _ := handler["async"].(bool)
			if asynchronous && numberMapValue(handler, "timeout") == claudeCodeHookTimeoutSeconds {
				return true
			}
		}
	}
	return false
}

func isAiguardHookHandler(handler map[string]any) bool {
	handlerType, _ := handler["type"].(string)
	if handlerType != "command" {
		return false
	}
	arguments := stringArrayValue(handler["args"])
	agentFlag := slices.Index(arguments, "--agent")
	return len(arguments) > 0 && arguments[0] == agenthook.Subcommand &&
		agentFlag >= 0 && agentFlag+1 < len(arguments) && arguments[agentFlag+1] == "claude-code" &&
		slices.Contains(arguments, "--records-url")
}

// removeClaudeCodeHooks strips only aiguard command handlers from the current
// settings. Other handlers in the same group, groups added later, and every
// unrelated setting remain untouched.
func removeClaudeCodeHooks(config map[string]any) bool {
	hooks, ok := objectValue(config["hooks"])
	if !ok {
		return false
	}
	changed := false

	for _, eventName := range claudeCodeHookEvents {
		rawGroups, exists := hooks[eventName]
		if !exists {
			continue
		}
		groups, ok := rawGroups.([]any)
		if !ok {
			continue
		}
		keptGroups := make([]any, 0, len(groups))
		eventChanged := false
		for _, rawGroup := range groups {
			group, ok := objectValue(rawGroup)
			if !ok {
				keptGroups = append(keptGroups, rawGroup)
				continue
			}
			handlers, ok := group["hooks"].([]any)
			if !ok {
				keptGroups = append(keptGroups, rawGroup)
				continue
			}
			keptHandlers := make([]any, 0, len(handlers))
			groupChanged := false
			for _, rawHandler := range handlers {
				handler, ok := objectValue(rawHandler)
				if ok && isAiguardHookHandler(handler) {
					eventChanged = true
					groupChanged = true
					continue
				}
				keptHandlers = append(keptHandlers, rawHandler)
			}
			if len(keptHandlers) == 0 && groupChanged {
				continue
			}
			if groupChanged {
				group["hooks"] = keptHandlers
			}
			keptGroups = append(keptGroups, group)
		}
		if !eventChanged {
			continue
		}
		changed = true
		if len(keptGroups) > 0 {
			hooks[eventName] = keptGroups
		} else {
			delete(hooks, eventName)
		}
	}
	if changed && len(hooks) == 0 {
		delete(config, "hooks")
	}
	return changed
}

func objectValue(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	return object, ok
}

func numberMapValue(object map[string]any, key string) int {
	switch value := object[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 0
	}
}

func stringArrayValue(value any) []string {
	switch array := value.(type) {
	case []string:
		return array
	case []any:
		result := make([]string, 0, len(array))
		for _, item := range array {
			text, ok := item.(string)
			if !ok {
				return nil
			}
			result = append(result, text)
		}
		return result
	default:
		return nil
	}
}
