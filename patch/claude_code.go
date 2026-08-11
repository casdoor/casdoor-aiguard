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
	"sort"
	"strings"

	"github.com/casdoor/casdoor-aiguard/agenthook"
	"github.com/casdoor/casdoor-aiguard/conf"
	"github.com/casdoor/casdoor-aiguard/util"
)

const (
	claudeCodeHookTimeoutSeconds = 5
	claudeCodeClaimsFlag         = "--aiguard-claims"
)

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

func (claudeCodePatcher) Patch(target Target) error {
	return updateClaudeCodeHooks(target, true)
}

func (claudeCodePatcher) Unpatch(target Target) error {
	return updateClaudeCodeHooks(target, false)
}

func (claudeCodePatcher) Status(target Target) (Status, error) {
	active, detail, err := claudeCodeHooksStatus(target)
	return Status{Patched: active, Detail: detail}, err
}

func claudeCodeConfigPath(target Target) (string, error) {
	home, err := homeOf(target)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

func claudeCodeHookCommand(claims map[string]struct{}) (string, []string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", nil, fmt.Errorf("cannot resolve the aiguard binary to register: %w", err)
	}
	names := make([]string, 0, len(claims))
	for claim := range claims {
		names = append(names, claim)
	}
	sort.Strings(names)
	return executable, []string{
		agenthook.Subcommand,
		"--agent", "claude-code",
		"--records-url", conf.GetRecordsIngestUrl(),
		claudeCodeClaimsFlag, strings.Join(names, ","),
	}, nil
}

func updateClaudeCodeHooks(target Target, add bool) error {
	configPath, err := claudeCodeConfigPath(target)
	if err != nil {
		return err
	}
	config, mode, exists, err := readJSONConfigFile(configPath)
	if err != nil {
		return err
	}
	if !add && !exists {
		return nil
	}
	claims := claudeCodeHookClaims(config)
	if add {
		claims[target.AgentId] = struct{}{}
	} else {
		if _, ok := claims[target.AgentId]; !ok {
			return nil
		}
		delete(claims, target.AgentId)
	}

	changed := false
	if len(claims) == 0 {
		changed = removeClaudeCodeHooks(config)
	} else {
		command, arguments, err := claudeCodeHookCommand(claims)
		if err != nil {
			return err
		}
		changed, err = installClaudeCodeHooks(config, command, arguments)
		if err != nil {
			return fmt.Errorf("cannot merge Claude Code hooks: %w", err)
		}
	}
	if !changed {
		return nil
	}
	if add {
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			return err
		}
	}
	return writeJSONConfigFile(configPath, config, mode)
}

func claudeCodeHooksStatus(target Target) (bool, string, error) {
	configPath, err := claudeCodeConfigPath(target)
	if err != nil {
		return false, "", err
	}
	config, _, exists, err := readJSONConfigFile(configPath)
	if err != nil {
		return false, "", err
	}
	if !exists {
		return false, "Code hooks missing", nil
	}
	claims := claudeCodeHookClaims(config)
	if _, ok := claims[target.AgentId]; !ok {
		return false, "Code hooks missing", nil
	}
	command, arguments, err := claudeCodeHookCommand(claims)
	if err != nil {
		return false, "", err
	}
	owned, current := claudeCodeHooksState(config, command, arguments)
	if current < len(claudeCodeHookEvents) {
		return false, fmt.Sprintf("Code hooks need refresh (%d/%d installed, %d/%d current)",
			owned, len(claudeCodeHookEvents), current, len(claudeCodeHookEvents)), nil
	}
	return true, "Code hooks active", nil
}

func claudeCodeHookClaims(config map[string]any) map[string]struct{} {
	claims := map[string]struct{}{}
	hooks, _ := objectValue(config["hooks"])
	owned := false
	for _, eventName := range claudeCodeHookEvents {
		groups, _ := hooks[eventName].([]any)
		for _, rawGroup := range groups {
			group, _ := objectValue(rawGroup)
			handlers, _ := group["hooks"].([]any)
			for _, rawHandler := range handlers {
				handler, ok := objectValue(rawHandler)
				if !ok || !isAiguardHookHandler(handler) {
					continue
				}
				owned = true
				arguments := stringArrayValue(handler["args"])
				index := slices.Index(arguments, claudeCodeClaimsFlag)
				if index < 0 || index+1 >= len(arguments) {
					continue
				}
				for _, claim := range strings.Split(arguments[index+1], ",") {
					if claim == "claude-code" || claim == "claude-desktop" {
						claims[claim] = struct{}{}
					}
				}
			}
		}
	}
	if owned && len(claims) == 0 {
		claims["claude-code"] = struct{}{}
	}
	return claims
}

// readJSONConfigFile reads a JSON object config file, reporting a missing
// file as an empty object rather than an error so callers can create it, and
// the file's pre-existing permission bits so writeJSONConfigFile can preserve
// them. Nothing here is Claude-specific: Codex's auth.json is plain JSON too,
// so codex_llm.go reuses this rather than duplicating it.
func readJSONConfigFile(path string) (map[string]any, os.FileMode, bool, error) {
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

func writeJSONConfigFile(path string, config map[string]any, mode os.FileMode) error {
	updated, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return util.AtomicWriteFile(path, append(updated, '\n'), mode)
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
