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
	"strings"

	"github.com/casdoor/casdoor-aiguard/claudecodehook"
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

	return Apply(target, func(changes *ChangeSet) error {
		if err := changes.MkdirAll(filepath.Dir(configPath)); err != nil {
			return err
		}
		config, err := readJSONObject(changes, configPath)
		if err != nil {
			return err
		}
		if err := installClaudeCodeHooks(config, hookCommand, hookArguments); err != nil {
			return fmt.Errorf("cannot merge Claude Code hooks: %w", err)
		}
		configureClaudeCodeTelemetry(config, conf.GetOtelLogsIngestUrl())

		updated, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return err
		}
		return changes.WriteFile(configPath, append(updated, '\n'), 0o600)
	})
}

func (claudeCodePatcher) Unpatch(target Target) error {
	return Revert(target)
}

func (p claudeCodePatcher) Status(target Target) (Status, error) {
	configPath, err := p.configPath(target)
	if err != nil {
		return Status{}, err
	}
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return Status{Detail: "not patched"}, nil
	}
	if err != nil {
		return Status{}, err
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return Status{}, fmt.Errorf("cannot parse %s: %w", configPath, err)
	}

	hookState := claudeCodeHooksState(config)
	if hookState == 0 {
		return Status{Detail: "not patched"}, nil
	}
	if hookState < len(claudeCodeHookEvents) {
		return Status{Detail: fmt.Sprintf("Claude Code hooks are incomplete (%d/%d active)", hookState, len(claudeCodeHookEvents))}, nil
	}

	detail := "Hooks active, LLM telemetry unavailable"
	if claudeCodeTelemetryActive(config, conf.GetOtelLogsIngestUrl()) {
		detail = "Hooks and LLM telemetry active; start a new Claude Code session after patching"
	}
	if !IsApplied(target) {
		detail += "; patched outside aiguard, so unpatch cannot restore the original settings"
	}
	return Status{Patched: true, Detail: detail}, nil
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
		claudecodehook.Subcommand,
		"--records-url", conf.GetRecordsIngestUrl(),
		"--agent-path", target.Path,
	}, nil
}

func installClaudeCodeHooks(config map[string]any, command string, arguments []string) error {
	hooks, ok := objectValue(config["hooks"])
	if config["hooks"] != nil && !ok {
		return fmt.Errorf("hooks must be a JSON object")
	}
	if !ok {
		hooks = map[string]any{}
		config["hooks"] = hooks
	}

	for _, eventName := range claudeCodeHookEvents {
		if eventHasAiguardHook(hooks[eventName]) {
			continue
		}
		groups, ok := arrayValue(hooks[eventName])
		if hooks[eventName] != nil && !ok {
			return fmt.Errorf("hooks.%s must be a JSON array", eventName)
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
	}
	return nil
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
	groups, ok := arrayValue(value)
	if !ok {
		return false
	}
	for _, rawGroup := range groups {
		group, ok := objectValue(rawGroup)
		if !ok {
			continue
		}
		handlers, ok := arrayValue(group["hooks"])
		if !ok {
			continue
		}
		for _, rawHandler := range handlers {
			handler, ok := objectValue(rawHandler)
			if !ok || stringMapValue(handler, "type") != "command" {
				continue
			}
			command := stringMapValue(handler, "command")
			arguments := stringArrayValue(handler["args"])
			execForm := len(arguments) > 0 && arguments[0] == claudecodehook.Subcommand && stringSliceContains(arguments, "--records-url")
			legacyShellForm := strings.Contains(command, claudecodehook.Subcommand) && strings.Contains(command, "--records-url")
			asynchronous, _ := handler["async"].(bool)
			if (execForm || legacyShellForm) &&
				asynchronous && numberMapValue(handler, "timeout") == claudeCodeHookTimeoutSeconds {
				return true
			}
		}
	}
	return false
}

// configureClaudeCodeTelemetry augments only a configuration that does not
// already select an external logs collector. It returns false when telemetry
// must remain untouched, while hooks continue to provide behavioural auditing.
func configureClaudeCodeTelemetry(config map[string]any, endpoint string) bool {
	environment, ok := objectValue(config["env"])
	if config["env"] != nil && !ok {
		return false
	}
	if !ok {
		environment = map[string]any{}
		config["env"] = environment
	}

	if stringMapValue(environment, "CLAUDE_CODE_ENABLE_TELEMETRY") == "0" {
		return false
	}
	exporters := splitExporters(stringMapValue(environment, "OTEL_LOGS_EXPORTER"))
	if exporters["none"] {
		return false
	}
	logsEndpoint := stringMapValue(environment, "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT")
	globalEndpoint := stringMapValue(environment, "OTEL_EXPORTER_OTLP_ENDPOINT")
	protocol := stringMapValue(environment, "OTEL_EXPORTER_OTLP_LOGS_PROTOCOL")

	consoleOnly := len(exporters) == 1 && exporters["console"]
	if exporters["otlp"] && logsEndpoint == "" && globalEndpoint == "" {
		return false
	}
	usesOtlp := exporters["otlp"] || (!consoleOnly && (logsEndpoint != "" || globalEndpoint != ""))
	if usesOtlp && !telemetryEndpointMatches(logsEndpoint, globalEndpoint, endpoint) && (logsEndpoint != "" || globalEndpoint != "") {
		return false
	}
	if usesOtlp && protocol != "" && protocol != "http/json" {
		return false
	}
	for exporter := range exporters {
		if exporter != "console" && exporter != "otlp" {
			return false
		}
	}

	environment["CLAUDE_CODE_ENABLE_TELEMETRY"] = "1"
	if exporters["console"] {
		environment["OTEL_LOGS_EXPORTER"] = "console,otlp"
	} else {
		environment["OTEL_LOGS_EXPORTER"] = "otlp"
	}
	environment["OTEL_EXPORTER_OTLP_LOGS_PROTOCOL"] = "http/json"
	environment["OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"] = endpoint
	environment["OTEL_LOG_TOOL_DETAILS"] = "1"
	environment["OTEL_LOG_ASSISTANT_RESPONSES"] = "0"
	return true
}

func claudeCodeTelemetryActive(config map[string]any, endpoint string) bool {
	environment, ok := objectValue(config["env"])
	if !ok || stringMapValue(environment, "CLAUDE_CODE_ENABLE_TELEMETRY") == "0" {
		return false
	}
	exporters := splitExporters(stringMapValue(environment, "OTEL_LOGS_EXPORTER"))
	if !exporters["otlp"] {
		return false
	}
	protocol := stringMapValue(environment, "OTEL_EXPORTER_OTLP_LOGS_PROTOCOL")
	if protocol == "" {
		protocol = stringMapValue(environment, "OTEL_EXPORTER_OTLP_PROTOCOL")
	}
	if protocol != "http/json" {
		return false
	}
	return telemetryEndpointMatches(
		stringMapValue(environment, "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"),
		stringMapValue(environment, "OTEL_EXPORTER_OTLP_ENDPOINT"),
		endpoint,
	)
}

func telemetryEndpointMatches(logsEndpoint string, globalEndpoint string, desired string) bool {
	desired = strings.TrimRight(desired, "/")
	if strings.TrimRight(logsEndpoint, "/") == desired && logsEndpoint != "" {
		return true
	}
	derived := strings.TrimRight(globalEndpoint, "/") + "/v1/logs"
	return globalEndpoint != "" && derived == desired
}

func splitExporters(value string) map[string]bool {
	result := map[string]bool{}
	for _, exporter := range strings.Split(value, ",") {
		if exporter = strings.ToLower(strings.TrimSpace(exporter)); exporter != "" {
			result[exporter] = true
		}
	}
	return result
}

func objectValue(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	return object, ok
}

func arrayValue(value any) ([]any, bool) {
	if value == nil {
		return []any{}, true
	}
	array, ok := value.([]any)
	return array, ok
}

func stringMapValue(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return value
}

func numberMapValue(object map[string]any, key string) int {
	switch value := object[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
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

func stringSliceContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
