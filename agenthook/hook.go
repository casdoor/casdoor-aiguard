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

// Package agenthook turns JSON command-hook invocations into aiguard behaviour
// records. Agent-specific event names and field layouts live in the profile
// table; input handling, sanitization and reporting are shared.
package agenthook

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/casdoor/casdoor-aiguard/auditutil"
	"github.com/casdoor/casdoor-aiguard/object"
)

const (
	Subcommand       = "agent-hook"
	maxHookInput     = 8 * 1024 * 1024
	maxPayloadBytes  = 64 * 1024
	reportTimeout    = 5 * time.Second
	recordTimeFormat = "2006-01-02T15:04:05.000Z07:00"
)

type eventRule struct {
	eventType   string
	action      string
	outcome     string
	detailField string
	tool        bool
}

type hookProfile struct {
	eventField      string
	sessionField    string
	promptField     string
	toolUseField    string
	toolNameField   string
	modelField      string
	durationField   string
	mcpPrefix       string
	toolInputField  string
	toolOutputField string
	omitFields      map[string]struct{}
	lengthFields    map[string]struct{}
	events          map[string]eventRule
}

// profiles contains the only data that differs between compatible JSON hook
// agents. Adding one does not require another package or normalizer.
var profiles = map[string]hookProfile{
	"claude-code": {
		eventField:      "hook_event_name",
		sessionField:    "session_id",
		promptField:     "prompt_id",
		toolUseField:    "tool_use_id",
		toolNameField:   "tool_name",
		modelField:      "model",
		durationField:   "duration_ms",
		mcpPrefix:       "mcp__",
		toolInputField:  "tool_input",
		toolOutputField: "tool_response",
		omitFields: map[string]struct{}{
			"transcript_path": {},
		},
		lengthFields: map[string]struct{}{
			"last_assistant_message": {},
			"compact_summary":        {},
		},
		events: map[string]eventRule{
			"SessionStart":       {eventType: "session", action: "start"},
			"SessionEnd":         {eventType: "session", action: "end"},
			"Stop":               {eventType: "session", action: "stop", outcome: "success"},
			"StopFailure":        {eventType: "session", action: "stop", outcome: "failure", detailField: "error"},
			"UserPromptSubmit":   {eventType: "prompt", action: "submitted", outcome: "attempted"},
			"PreToolUse":         {eventType: "tool", action: "call", outcome: "attempted", tool: true},
			"PostToolUse":        {eventType: "tool", action: "call", outcome: "success", tool: true},
			"PostToolUseFailure": {eventType: "tool", action: "call", outcome: "failure", detailField: "error", tool: true},
			"PermissionRequest":  {eventType: "permission", action: "requested", outcome: "attempted"},
			"PermissionDenied":   {eventType: "permission", action: "denied", outcome: "denied", detailField: "reason"},
			"SubagentStart":      {eventType: "subagent", action: "start"},
			"SubagentStop":       {eventType: "subagent", action: "stop", outcome: "success"},
			"PreCompact":         {eventType: "compact", action: "before", outcome: "attempted"},
			"PostCompact":        {eventType: "compact", action: "after", outcome: "success"},
		},
	},
}

// ServeIfInvoked handles the hook subcommand and exits successfully. Hook
// collection is deliberately best effort: invalid input and an unreachable
// aiguard must never change the invoking agent's behaviour.
func ServeIfInvoked() {
	if len(os.Args) < 2 || os.Args[1] != Subcommand {
		return
	}
	_ = Run(os.Args[2:], os.Stdin)
	os.Exit(0)
}

// Run reads one hook event, normalizes it and posts it to the Records ingest
// endpoint. It returns errors for unit tests and embedders; ServeIfInvoked
// intentionally discards them.
func Run(args []string, input io.Reader) error {
	flags := flag.NewFlagSet(Subcommand, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	agentId := flags.String("agent", "", "id of the agent that invoked the hook")
	recordsUrl := flags.String("records-url", "", "aiguard records ingest endpoint")
	agentPath := flags.String("agent-path", "", "path of the agent installation")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if _, ok := profiles[*agentId]; !ok {
		return fmt.Errorf("unsupported hook agent %q", *agentId)
	}

	decoder := json.NewDecoder(io.LimitReader(input, maxHookInput))
	decoder.UseNumber()
	var event map[string]any
	if err := decoder.Decode(&event); err != nil {
		return err
	}
	record := Normalize(*agentId, event, *agentPath, time.Now())
	if record == nil || *recordsUrl == "" {
		return nil
	}

	body, err := json.Marshal(record)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, *recordsUrl, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: reportTimeout}).Do(request)
	if err != nil {
		return err
	}
	return response.Body.Close()
}

// Normalize maps one registered agent's hook vocabulary onto aiguard's stable
// record taxonomy. Unknown agents and events are ignored.
func Normalize(agentId string, event map[string]any, agentPath string, now time.Time) *object.Record {
	profile, ok := profiles[agentId]
	if !ok {
		return nil
	}
	rule, ok := profile.events[stringValue(event[profile.eventField])]
	if !ok {
		return nil
	}
	record := &object.Record{
		Agent:       agentId,
		AgentPath:   agentPath,
		CreatedTime: now.Format(recordTimeFormat),
		SessionKey:  stringValue(event[profile.sessionField]),
		PromptId:    stringValue(event[profile.promptField]),
		ToolUseId:   stringValue(event[profile.toolUseField]),
		ToolName:    stringValue(event[profile.toolNameField]),
		Model:       stringValue(event[profile.modelField]),
		DurationMs:  int64Value(event[profile.durationField]),
		EventType:   rule.eventType,
		Action:      rule.action,
		Outcome:     rule.outcome,
	}
	if rule.detailField != "" {
		record.Detail = auditutil.SanitizeString(stringValue(event[rule.detailField]))
	}
	if isMcp := setMcpTarget(record, profile.mcpPrefix); rule.tool && isMcp {
		record.EventType = "mcp"
	}

	payload := preparePayload(profile, event, record.ToolName)
	record.Object = encodePayload(payload)
	return record
}

func setMcpTarget(record *object.Record, prefix string) bool {
	server, tool, ok := parseMcpTool(record.ToolName, prefix)
	if ok {
		record.McpServer = server
		record.McpTool = tool
	}
	return ok
}

func parseMcpTool(name string, prefix string) (string, string, bool) {
	if prefix == "" || !strings.HasPrefix(name, prefix) {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(name, prefix), "__", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func preparePayload(profile hookProfile, event map[string]any, toolName string) map[string]any {
	copy := make(map[string]any, len(event))
	for key, value := range event {
		if _, omit := profile.omitFields[key]; omit {
			continue
		}
		if _, lengthOnly := profile.lengthFields[key]; lengthOnly {
			copy[key+"_length"] = len(stringValue(value))
			continue
		}
		copy[key] = auditutil.SanitizeValue(key, value)
	}

	if profile.toolInputField != "" && auditutil.IsSensitiveRead(toolName, event[profile.toolInputField]) {
		if _, ok := copy[profile.toolOutputField]; ok {
			copy[profile.toolOutputField] = "[REDACTED: sensitive file content]"
		}
	}
	return copy
}

func encodePayload(payload map[string]any) string {
	return auditutil.EncodeBoundedJSON(payload, maxPayloadBytes)
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case json.Number:
		result, _ := typed.Int64()
		return result
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	default:
		return 0
	}
}
