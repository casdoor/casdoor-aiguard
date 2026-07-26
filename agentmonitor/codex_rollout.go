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

package agentmonitor

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/casdoor/casdoor-aiguard/auditutil"
	"github.com/casdoor/casdoor-aiguard/object"
)

const codexRecordTimeFormat = "2006-01-02T15:04:05.000Z07:00"

type codexRolloutEntry struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexRolloutPayload struct {
	ID         string `json:"id"`
	Originator string `json:"originator"`
	TurnID     string `json:"turn_id"`
	Model      string `json:"model"`
	Type       string `json:"type"`
	Role       string `json:"role"`
	Name       string `json:"name"`
	CallID     string `json:"call_id"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Info struct {
		LastTokenUsage struct {
			Input           json.Number `json:"input_tokens"`
			CachedInput     json.Number `json:"cached_input_tokens"`
			Output          json.Number `json:"output_tokens"`
			ReasoningOutput json.Number `json:"reasoning_output_tokens"`
			Total           json.Number `json:"total_tokens"`
		} `json:"last_token_usage"`
		ContextWindow json.Number `json:"model_context_window"`
	} `json:"info"`
	Invocation struct {
		Server string `json:"server"`
		Tool   string `json:"tool"`
	} `json:"invocation"`
	Result map[string]json.RawMessage `json:"result"`
}

func parseCodexHeader(line []byte) codexHeaderMeta {
	entry, payload, ok := decodeCodexLine(line)
	if !ok || entry.Type != "session_meta" {
		return codexHeaderMeta{}
	}
	return codexHeaderMeta{SessionKey: payload.ID, AgentID: codexAgentForSource(payload.Originator)}
}

func parseCodexRolloutLine(line []byte, cursor *codexCursor, claim *codexClaim) []*object.Record {
	if cursor == nil {
		return nil
	}
	if cursor.Pending == nil {
		cursor.Pending = map[string]codexPendingCall{}
	}
	entry, payload, ok := decodeCodexLine(line)
	if !ok {
		return nil
	}
	when := codexTimestamp(entry.Timestamp)

	switch entry.Type {
	case "session_meta":
		cursor.SessionKey = payload.ID
		cursor.AgentID = codexAgentForSource(payload.Originator)
	case "turn_context":
		cursor.TurnID, cursor.Model = payload.TurnID, payload.Model
	case "event_msg":
		switch payload.Type {
		case "task_started", "task_complete", "turn_aborted":
			resetCodexCursorTurn(cursor)
		case "token_count":
			return codexUsageRecord(payload, cursor, claim, when)
		case "exec_command_end":
			return codexFinishTool(payload, cursor, claim, when, "exec")
		case "mcp_tool_call_end":
			return codexFinishTool(payload, cursor, claim, when, "mcp")
		}
	case "response_item":
		switch payload.Type {
		case "message":
			return codexMessageRecord(payload, cursor, claim, when)
		case "function_call", "custom_tool_call":
			codexRememberTool(payload, cursor, when)
		case "function_call_output", "custom_tool_call_output", "tool_search_output":
			return codexFinishTool(payload, cursor, claim, when, "")
		case "tool_search_call", "web_search_call", "image_generation_call", "local_shell_call":
			return codexHostedTool(payload, strings.TrimSuffix(payload.Type, "_call"), cursor, claim, when)
		}
	}
	return nil
}

func decodeCodexLine(line []byte) (codexRolloutEntry, codexRolloutPayload, bool) {
	var entry codexRolloutEntry
	var payload codexRolloutPayload
	if len(line) == 0 || json.Unmarshal(line, &entry) != nil || entry.Type == "" ||
		json.Unmarshal(entry.Payload, &payload) != nil {
		return entry, payload, false
	}
	return entry, payload, true
}

func codexAgentForSource(originator string) string {
	switch originator {
	case "codex_desktop":
		return "codex"
	case "codex-tui", "codex-exec":
		return "codex-cli"
	default:
		return ""
	}
}

func codexMessageRecord(payload codexRolloutPayload, cursor *codexCursor, claim *codexClaim, when time.Time) []*object.Record {
	if !codexClaimMatches(cursor, claim) || cursor.TurnID == "" {
		return nil
	}
	role := strings.ToLower(payload.Role)
	if role != "user" && role != "assistant" {
		return nil
	}
	length, images := 0, 0
	for _, item := range payload.Content {
		switch item.Type {
		case "input_text", "output_text":
			length += utf8.RuneCountInString(item.Text)
		case "input_image", "output_image":
			images++
		}
	}
	if length == 0 && images == 0 {
		return nil
	}
	action, outcome := "request", "attempted"
	if role == "assistant" {
		action, outcome = "response", "success"
	}
	body := map[string]any{"contentLength": length}
	if images > 0 {
		body["imageCount"] = images
	}
	record := codexBaseRecord(cursor, claim, when)
	record.EventType, record.Action, record.Outcome = "llm", action, outcome
	record.PromptId = cursor.TurnID
	record.Object = auditutil.EncodeBoundedJSON(body, 64*1024)
	return []*object.Record{record}
}

func codexUsageRecord(payload codexRolloutPayload, cursor *codexCursor, claim *codexClaim, when time.Time) []*object.Record {
	if !codexClaimMatches(cursor, claim) || cursor.TurnID == "" {
		return nil
	}
	usage := payload.Info.LastTokenUsage
	safe := map[string]any{}
	values := []struct {
		key   string
		value json.Number
	}{
		{"input_tokens", usage.Input}, {"cached_input_tokens", usage.CachedInput},
		{"output_tokens", usage.Output}, {"reasoning_output_tokens", usage.ReasoningOutput},
		{"total_tokens", usage.Total}, {"model_context_window", payload.Info.ContextWindow},
	}
	for _, value := range values {
		if value.value != "" {
			safe[value.key] = value.value
		}
	}
	if len(safe) == 0 {
		return nil
	}
	record := codexBaseRecord(cursor, claim, when)
	record.EventType, record.Action, record.Outcome = "llm", "usage", "success"
	record.PromptId = cursor.TurnID
	record.Object = auditutil.EncodeBoundedJSON(safe, 64*1024)
	return []*object.Record{record}
}

func codexRememberTool(payload codexRolloutPayload, cursor *codexCursor, when time.Time) {
	if payload.CallID != "" && payload.Name != "" {
		cursor.Pending[payload.CallID] = codexPendingCall{
			Name: payload.Name, StartedAt: when, TurnID: cursor.TurnID,
		}
	}
}

func codexHostedTool(
	payload codexRolloutPayload,
	name string,
	cursor *codexCursor,
	claim *codexClaim,
	when time.Time,
) []*object.Record {
	outcome, terminal := codexStatus(payload.Status)
	if payload.CallID == "" {
		if !terminal || !codexClaimMatches(cursor, claim) {
			return nil
		}
		call := codexPendingCall{Name: name, StartedAt: when, TurnID: cursor.TurnID}
		return codexToolPair(cursor, claim, "", call, when, outcome)
	}
	if _, exists := cursor.Pending[payload.CallID]; !exists {
		payload.Name = name
		codexRememberTool(payload, cursor, when)
	}
	if !terminal {
		return nil
	}
	return codexFinishTool(payload, cursor, claim, when, "")
}

func codexFinishTool(
	payload codexRolloutPayload,
	cursor *codexCursor,
	claim *codexClaim,
	when time.Time,
	kind string,
) []*object.Record {
	call, found := cursor.Pending[payload.CallID]
	if !found {
		return nil
	}
	delete(cursor.Pending, payload.CallID)
	if !codexClaimMatches(cursor, claim) {
		return nil
	}
	outcome, _ := codexStatus(payload.Status)
	if kind == "exec" && payload.ExitCode != 0 {
		outcome = "failure"
	}
	if kind == "mcp" {
		if payload.Invocation.Server != "" && payload.Invocation.Tool != "" {
			call.Name = "mcp__" + payload.Invocation.Server + "__" + payload.Invocation.Tool
		}
		if _, failed := payload.Result["Err"]; failed {
			outcome = "failure"
		}
	}
	return codexToolPair(cursor, claim, payload.CallID, call, when, outcome)
}

func codexToolPair(
	cursor *codexCursor,
	claim *codexClaim,
	callID string,
	call codexPendingCall,
	when time.Time,
	outcome string,
) []*object.Record {
	return []*object.Record{
		codexToolRecord(cursor, claim, call.StartedAt, callID, call, "call", "attempted"),
		codexToolRecord(cursor, claim, when, callID, call, "result", outcome),
	}
}

func codexToolRecord(
	cursor *codexCursor,
	claim *codexClaim,
	when time.Time,
	callID string,
	call codexPendingCall,
	action, outcome string,
) *object.Record {
	record := codexBaseRecord(cursor, claim, when)
	record.EventType, record.Action, record.Outcome = "tool", action, outcome
	record.ToolUseId, record.ToolName, record.PromptId = callID, call.Name, call.TurnID
	if server, tool, ok := auditutil.ParseMcpTool(call.Name, "mcp__"); ok {
		record.EventType, record.McpServer, record.McpTool = "mcp", server, tool
	}
	if action == "result" && !call.StartedAt.IsZero() {
		record.DurationMs = when.Sub(call.StartedAt).Milliseconds()
		if record.DurationMs < 0 {
			record.DurationMs = 0
		}
	}
	return record
}

func codexStatus(status string) (string, bool) {
	switch strings.ToLower(status) {
	case "failed", "error", "cancelled":
		return "failure", true
	case "denied":
		return "denied", true
	case "completed":
		return "success", true
	default:
		return "success", false
	}
}

func codexClaimMatches(cursor *codexCursor, claim *codexClaim) bool {
	return claim != nil && cursor.AgentID != "" && cursor.AgentID == claim.AgentID
}

func codexBaseRecord(cursor *codexCursor, claim *codexClaim, when time.Time) *object.Record {
	return &object.Record{
		CreatedTime: when.Format(codexRecordTimeFormat),
		Agent:       cursor.AgentID, AgentPath: claim.Path, User: claim.Owner,
		SessionKey: cursor.SessionKey, Model: cursor.Model,
	}
}

func codexTimestamp(value string) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed
	}
	return time.Now()
}
