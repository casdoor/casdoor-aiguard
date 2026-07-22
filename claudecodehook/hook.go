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

// Package claudecodehook turns one Claude Code hook invocation into an
// aiguard behaviour record. Claude Code starts the aiguard binary as an async
// command hook and sends the event JSON on stdin; this package sanitizes and
// bounds it before it ever leaves the machine.
package claudecodehook

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/casdoor/casdoor-aiguard/auditutil"
	"github.com/casdoor/casdoor-aiguard/object"
)

const (
	Subcommand       = "claude-code-hook"
	maxHookInput     = 8 * 1024 * 1024
	maxPayloadBytes  = 64 * 1024
	reportTimeout    = 5 * time.Second
	recordTimeFormat = "2006-01-02T15:04:05.000Z07:00"
)

// ServeIfInvoked handles the hook subcommand and exits successfully. Hook
// collection is deliberately best effort: invalid input and an unreachable
// aiguard must never change Claude Code's behaviour or surface as a hook error.
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
	recordsUrl := flags.String("records-url", "", "aiguard records ingest endpoint")
	agentPath := flags.String("agent-path", "", "path of the Claude Code installation")
	if err := flags.Parse(args); err != nil {
		return err
	}

	decoder := json.NewDecoder(io.LimitReader(input, maxHookInput))
	decoder.UseNumber()
	var event map[string]any
	if err := decoder.Decode(&event); err != nil {
		return err
	}
	record := Normalize(event, *agentPath, time.Now())
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

// Normalize maps Claude Code's versioned hook vocabulary onto aiguard's small,
// stable event taxonomy. Unknown events are ignored instead of becoming
// misleading records.
func Normalize(event map[string]any, agentPath string, now time.Time) *object.Record {
	eventName := stringValue(event["hook_event_name"])
	record := &object.Record{
		Agent:       "claude-code",
		AgentPath:   agentPath,
		CreatedTime: now.Format(recordTimeFormat),
		SessionKey:  stringValue(event["session_id"]),
		PromptId:    stringValue(event["prompt_id"]),
		ToolUseId:   stringValue(event["tool_use_id"]),
		ToolName:    stringValue(event["tool_name"]),
		Model:       stringValue(event["model"]),
		DurationMs:  int64Value(event["duration_ms"]),
	}

	switch eventName {
	case "SessionStart":
		record.EventType, record.Action = "session", "start"
	case "SessionEnd":
		record.EventType, record.Action = "session", "end"
	case "Stop":
		record.EventType, record.Action, record.Outcome = "session", "stop", "success"
	case "StopFailure":
		record.EventType, record.Action, record.Outcome = "session", "stop", "failure"
		record.Detail = auditutil.SanitizeString(stringValue(event["error"]))
	case "UserPromptSubmit":
		record.EventType, record.Action, record.Outcome = "prompt", "submitted", "attempted"
	case "PreToolUse":
		setToolEvent(record, "attempted")
	case "PostToolUse":
		setToolEvent(record, "success")
	case "PostToolUseFailure":
		setToolEvent(record, "failure")
		record.Detail = auditutil.SanitizeString(stringValue(event["error"]))
	case "PermissionRequest":
		record.EventType, record.Action, record.Outcome = "permission", "requested", "attempted"
	case "PermissionDenied":
		record.EventType, record.Action, record.Outcome = "permission", "denied", "denied"
		record.Detail = auditutil.SanitizeString(stringValue(event["reason"]))
	case "SubagentStart":
		record.EventType, record.Action = "subagent", "start"
	case "SubagentStop":
		record.EventType, record.Action, record.Outcome = "subagent", "stop", "success"
	case "PreCompact":
		record.EventType, record.Action, record.Outcome = "compact", "before", "attempted"
	case "PostCompact":
		record.EventType, record.Action, record.Outcome = "compact", "after", "success"
	default:
		return nil
	}
	setMcpTarget(record)

	payload := preparePayload(event, record.ToolName)
	record.Object = encodePayload(payload)
	return record
}

func setToolEvent(record *object.Record, outcome string) {
	record.EventType, record.Action, record.Outcome = "tool", "call", outcome
	if setMcpTarget(record) {
		record.EventType = "mcp"
	}
}

func setMcpTarget(record *object.Record) bool {
	server, tool, ok := parseMcpTool(record.ToolName)
	if ok {
		record.McpServer = server
		record.McpTool = tool
	}
	return ok
}

func parseMcpTool(name string) (string, string, bool) {
	if !strings.HasPrefix(name, "mcp__") {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(name, "mcp__"), "__", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func preparePayload(event map[string]any, toolName string) map[string]any {
	copy := make(map[string]any, len(event))
	for key, value := range event {
		switch key {
		case "transcript_path":
			continue
		case "last_assistant_message", "compact_summary":
			copy[key+"_length"] = len(stringValue(value))
			continue
		}
		copy[key] = auditutil.SanitizeValue(key, value)
	}

	if auditutil.IsSensitiveRead(toolName, event["tool_input"]) {
		if _, ok := copy["tool_response"]; ok {
			copy["tool_response"] = "[REDACTED: sensitive file content]"
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
