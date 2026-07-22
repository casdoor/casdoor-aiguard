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
	hookCommand, hookArguments, err := p.hookCommand(target)
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

	owned, current := claudeCodeHooksState(config, hookCommand, hookArguments)
	if owned == 0 {
		return Status{Detail: "not patched"}, nil
	}
	if current < len(claudeCodeHookEvents) {
		return Status{Detail: fmt.Sprintf("user settings hooks need refresh (%d/%d installed, %d/%d current)",
			owned, len(claudeCodeHookEvents), current, len(claudeCodeHookEvents))}, nil
	}

	return Status{Patched: true, Detail: "User settings hooks active"}, nil
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
		return config, info.Mode().Perm(), true, nil
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
		rawGroups, eventExists := hooks[eventName]
		groups, ok := rawGroups.([]any)
		if eventExists && !ok {
			return false, fmt.Errorf("hooks.%s must be a JSON array", eventName)
		}
		groups, eventChanged, found := mergeClaudeCodeHook(groups, command, arguments)
		if !found {
			group := map[string]any{"hooks": []any{newClaudeCodeHookHandler(command, arguments)}}
			if hookEventSupportsMatcher(eventName) {
				group["matcher"] = ""
			}
			groups = append(groups, group)
			eventChanged = true
		}
		if eventChanged {
			hooks[eventName] = groups
			changed = true
		}
	}
	return changed, nil
}

func newClaudeCodeHookHandler(command string, arguments []string) map[string]any {
	return map[string]any{
		"type":    "command",
		"command": command,
		"args":    arguments,
		"async":   true,
		"timeout": claudeCodeHookTimeoutSeconds,
	}
}

// mergeClaudeCodeHook refreshes the first owned handler in place and removes
// duplicates. Handler identity deliberately ignores mutable configuration;
// command, URL, async and timeout drift are corrected instead of being mistaken
// for a different handler.
func mergeClaudeCodeHook(groups []any, command string, arguments []string) ([]any, bool, bool) {
	result := make([]any, 0, len(groups))
	found := false
	changed := false

	for _, rawGroup := range groups {
		group, ok := objectValue(rawGroup)
		if !ok {
			result = append(result, rawGroup)
			continue
		}
		handlers, ok := group["hooks"].([]any)
		if !ok {
			result = append(result, rawGroup)
			continue
		}

		kept := make([]any, 0, len(handlers))
		groupChanged := false
		for _, rawHandler := range handlers {
			handler, ok := objectValue(rawHandler)
			if !ok || !isAiguardHookHandler(handler) {
				kept = append(kept, rawHandler)
				continue
			}
			if found {
				changed = true
				groupChanged = true
				continue
			}
			found = true
			if claudeCodeHookHandlerCurrent(handler, command, arguments) {
				kept = append(kept, rawHandler)
				continue
			}
			kept = append(kept, newClaudeCodeHookHandler(command, arguments))
			changed = true
			groupChanged = true
		}
		if len(kept) == 0 && groupChanged {
			continue
		}
		if groupChanged {
			group["hooks"] = kept
		}
		result = append(result, rawGroup)
	}
	return result, changed, found
}

func hookEventSupportsMatcher(eventName string) bool {
	switch eventName {
	case "PreToolUse", "PostToolUse", "PostToolUseFailure", "PermissionRequest", "PermissionDenied":
		return true
	default:
		return false
	}
}

func claudeCodeHooksState(config map[string]any, command string, arguments []string) (int, int) {
	hooks, ok := objectValue(config["hooks"])
	if !ok {
		return 0, 0
	}
	owned := 0
	current := 0
	for _, eventName := range claudeCodeHookEvents {
		eventOwned, eventCurrent := claudeCodeHookState(hooks[eventName], command, arguments)
		if eventOwned {
			owned++
		}
		if eventCurrent {
			current++
		}
	}
	return owned, current
}

func claudeCodeHookState(value any, command string, arguments []string) (bool, bool) {
	groups, ok := value.([]any)
	if !ok {
		return false, false
	}
	owned := 0
	current := false
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
			owned++
			if claudeCodeHookHandlerCurrent(handler, command, arguments) {
				current = true
			}
		}
	}
	return owned > 0, owned == 1 && current
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

func claudeCodeHookHandlerCurrent(handler map[string]any, command string, arguments []string) bool {
	configuredCommand, _ := handler["command"].(string)
	asynchronous, _ := handler["async"].(bool)
	return isAiguardHookHandler(handler) && configuredCommand == command &&
		slices.Equal(stringArrayValue(handler["args"]), arguments) && asynchronous &&
		numberMapValue(handler, "timeout") == claudeCodeHookTimeoutSeconds
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
