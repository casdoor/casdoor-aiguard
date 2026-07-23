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
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/casdoor/casdoor-aiguard/agenthook"
	"github.com/casdoor/casdoor-aiguard/conf"
)

const claudeCodeHookTimeoutSeconds = 5

type commandHookEvent struct {
	name    string
	matcher bool
}

type commandHookProfile struct {
	agentId    string
	configPath string
	events     []commandHookEvent
	timeout    int
}

var claudeCodeCommandHooks = commandHookProfile{
	agentId:    "claude-code",
	configPath: ".claude/settings.json",
	timeout:    claudeCodeHookTimeoutSeconds,
	events: []commandHookEvent{
		{name: "SessionStart"},
		{name: "UserPromptSubmit"},
		{name: "PreToolUse", matcher: true},
		{name: "PostToolUse", matcher: true},
		{name: "PostToolUseFailure", matcher: true},
		{name: "PermissionRequest", matcher: true},
		{name: "PermissionDenied", matcher: true},
		{name: "SubagentStart"},
		{name: "SubagentStop"},
		{name: "PreCompact"},
		{name: "PostCompact"},
		{name: "Stop"},
		{name: "StopFailure"},
		{name: "SessionEnd"},
	},
}

type commandHookPatcher struct {
	profile *commandHookProfile
}

func init() {
	register(commandHookPatcher{profile: &claudeCodeCommandHooks})
}

func (p commandHookPatcher) AgentId() string { return p.profile.agentId }

func (commandHookPatcher) Supported() bool { return true }

func (p commandHookPatcher) Patch(target Target) error {
	configPath, err := p.path(target)
	if err != nil {
		return err
	}
	command, arguments, err := p.command(target)
	if err != nil {
		return err
	}
	config, mode, _, err := readJSONConfig(configPath)
	if err != nil {
		return err
	}
	changed, err := mergeCommandHooks(p.profile, config, command, arguments)
	if err != nil {
		return fmt.Errorf("cannot merge %s hooks: %w", p.AgentId(), err)
	}
	if !changed {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	return writeJSONConfig(configPath, config, mode)
}

func (p commandHookPatcher) Unpatch(target Target) error {
	configPath, err := p.path(target)
	if err != nil {
		return err
	}
	config, mode, exists, err := readJSONConfig(configPath)
	if err != nil || !exists {
		return err
	}
	if !removeCommandHooks(p.profile, config) {
		return nil
	}
	return writeJSONConfig(configPath, config, mode)
}

func (p commandHookPatcher) Status(target Target) (Status, error) {
	configPath, err := p.path(target)
	if err != nil {
		return Status{}, err
	}
	command, arguments, err := p.command(target)
	if err != nil {
		return Status{}, err
	}
	config, _, exists, err := readJSONConfig(configPath)
	if err != nil {
		return Status{}, err
	}
	if !exists {
		return Status{Detail: "not patched"}, nil
	}

	owned, current := commandHooksState(p.profile, config, command, arguments)
	total := len(p.profile.events)
	if owned == 0 {
		return Status{Detail: "not patched"}, nil
	}
	if current < total {
		return Status{Detail: fmt.Sprintf("user settings hooks need refresh (%d/%d installed, %d/%d current)",
			owned, total, current, total)}, nil
	}
	return Status{Patched: true, Detail: "User settings hooks active"}, nil
}

func (p commandHookPatcher) path(target Target) (string, error) {
	home, err := homeOf(target)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, filepath.FromSlash(p.profile.configPath)), nil
}

func (p commandHookPatcher) command(target Target) (string, []string, error) {
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

func mergeCommandHooks(profile *commandHookProfile, config map[string]any, command string, arguments []string) (bool, error) {
	rawHooks, exists := config["hooks"]
	hooks, ok := objectValue(rawHooks)
	if exists && !ok {
		return false, fmt.Errorf("hooks must be a JSON object")
	}
	if !exists {
		hooks = map[string]any{}
		config["hooks"] = hooks
	}

	changed := false
	for _, event := range profile.events {
		rawGroups, exists := hooks[event.name]
		groups, ok := rawGroups.([]any)
		if exists && !ok {
			return false, fmt.Errorf("hooks.%s must be a JSON array", event.name)
		}

		desired := map[string]any{
			"type":    "command",
			"command": command,
			"args":    arguments,
			"async":   true,
			"timeout": profile.timeout,
		}
		result := make([]any, 0, len(groups)+1)
		found := false
		eventChanged := false
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
				if !ok || !profile.owns(handler) {
					kept = append(kept, rawHandler)
					continue
				}
				if found {
					eventChanged, groupChanged = true, true
					continue
				}
				found = true
				if commandHookCurrent(profile, handler, command, arguments) {
					kept = append(kept, rawHandler)
					continue
				}
				kept = append(kept, desired)
				eventChanged, groupChanged = true, true
			}
			if len(kept) == 0 && groupChanged {
				continue
			}
			if groupChanged {
				group["hooks"] = kept
			}
			result = append(result, rawGroup)
		}
		if !found {
			group := map[string]any{"hooks": []any{desired}}
			if event.matcher {
				group["matcher"] = ""
			}
			result = append(result, group)
			eventChanged = true
		}
		if eventChanged {
			hooks[event.name] = result
			changed = true
		}
	}
	return changed, nil
}

func commandHooksState(profile *commandHookProfile, config map[string]any, command string, arguments []string) (int, int) {
	hooks, ok := objectValue(config["hooks"])
	if !ok {
		return 0, 0
	}
	owned, current := 0, 0
	for _, event := range profile.events {
		groups, ok := hooks[event.name].([]any)
		if !ok {
			continue
		}
		ownedHandlers := 0
		eventCurrent := false
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
				if !ok || !profile.owns(handler) {
					continue
				}
				ownedHandlers++
				eventCurrent = eventCurrent || commandHookCurrent(profile, handler, command, arguments)
			}
		}
		if ownedHandlers > 0 {
			owned++
		}
		if ownedHandlers == 1 && eventCurrent {
			current++
		}
	}
	return owned, current
}

func (profile *commandHookProfile) owns(handler map[string]any) bool {
	handlerType, _ := handler["type"].(string)
	if handlerType != "command" {
		return false
	}
	arguments := stringArrayValue(handler["args"])
	agentFlag := slices.Index(arguments, "--agent")
	return len(arguments) > 0 && arguments[0] == agenthook.Subcommand &&
		agentFlag >= 0 && agentFlag+1 < len(arguments) && arguments[agentFlag+1] == profile.agentId &&
		slices.Contains(arguments, "--records-url")
}

func commandHookCurrent(profile *commandHookProfile, handler map[string]any, command string, arguments []string) bool {
	configuredCommand, _ := handler["command"].(string)
	asynchronous, _ := handler["async"].(bool)
	timeout := 0
	switch value := handler["timeout"].(type) {
	case int:
		timeout = value
	case float64:
		timeout = int(value)
	}
	return profile.owns(handler) && configuredCommand == command &&
		slices.Equal(stringArrayValue(handler["args"]), arguments) && asynchronous &&
		timeout == profile.timeout
}

func removeCommandHooks(profile *commandHookProfile, config map[string]any) bool {
	hooks, ok := objectValue(config["hooks"])
	if !ok {
		return false
	}
	changed := false

	for _, event := range profile.events {
		groups, ok := hooks[event.name].([]any)
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
			for _, rawHandler := range handlers {
				handler, ok := objectValue(rawHandler)
				if ok && profile.owns(handler) {
					eventChanged = true
					continue
				}
				keptHandlers = append(keptHandlers, rawHandler)
			}
			if len(keptHandlers) > 0 {
				group["hooks"] = keptHandlers
				keptGroups = append(keptGroups, group)
			} else if len(handlers) == 0 {
				keptGroups = append(keptGroups, group)
			}
		}
		if !eventChanged {
			continue
		}
		changed = true
		if len(keptGroups) == 0 {
			delete(hooks, event.name)
		} else {
			hooks[event.name] = keptGroups
		}
	}
	if changed && len(hooks) == 0 {
		delete(config, "hooks")
	}
	return changed
}
