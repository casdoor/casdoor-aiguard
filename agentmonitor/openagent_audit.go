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

	"github.com/casdoor/casdoor-aiguard/auditutil"
	"github.com/casdoor/casdoor-aiguard/object"
)

// This parser reads OpenAgent's append-only audit log. Each line is one
// self-contained JSON event; the schema is the one proposed upstream (a tool
// call or LLM call plus the guard verdict), so the fields here track that
// proposal and are read defensively - an unknown type or a missing field yields
// no record rather than an error.

const openAgentRecordTimeFormat = "2006-01-02T15:04:05.000Z07:00"

type openAgentAuditEntry struct {
	Timestamp       string `json:"timestamp"`
	SessionID       string `json:"sessionId"`
	Type            string `json:"type"`
	Tool            string `json:"tool"`
	Server          string `json:"server"`
	Model           string `json:"model"`
	ArgumentsLength int    `json:"argumentsLength"`
	ContentLength   int    `json:"contentLength"`
	Effect          string `json:"effect"`
	Reason          string `json:"reason"`
	Rule            string `json:"rule"`
	Outcome         string `json:"outcome"`
	Action          string `json:"action"`
	DurationMs      int64  `json:"durationMs"`
}

func parseOpenAgentAuditLine(line []byte, cursor *openAgentCursor, claim *openAgentClaim) []*object.Record {
	if cursor == nil || claim == nil || len(line) == 0 {
		return nil
	}
	var entry openAgentAuditEntry
	if json.Unmarshal(line, &entry) != nil || entry.Type == "" {
		return nil
	}
	if entry.SessionID != "" {
		cursor.SessionKey = entry.SessionID
	}
	when := openAgentTimestamp(entry.Timestamp)

	switch entry.Type {
	case "tool_call":
		return openAgentToolRecord(entry, cursor, claim, when)
	case "llm_call":
		return openAgentLlmRecord(entry, cursor, claim, when)
	case "session":
		return openAgentSessionRecord(entry, cursor, claim, when)
	default:
		return nil
	}
}

func openAgentToolRecord(entry openAgentAuditEntry, cursor *openAgentCursor, claim *openAgentClaim, when time.Time) []*object.Record {
	if entry.Tool == "" {
		return nil
	}
	record := openAgentBaseRecord(cursor, claim, when)
	record.EventType, record.Action = "tool", "call"
	record.Outcome = openAgentOutcome(entry.Outcome, entry.Effect)
	record.Model = entry.Model
	record.ToolName = entry.Tool
	record.DurationMs = openAgentDuration(entry.DurationMs)
	if entry.Server != "" {
		record.EventType = "mcp"
		record.McpServer = entry.Server
		record.McpTool = entry.Tool
		record.ToolName = "mcp__" + entry.Server + "__" + entry.Tool
	}

	body := map[string]any{}
	if entry.ArgumentsLength > 0 {
		body["argumentsLength"] = entry.ArgumentsLength
	}
	if entry.Effect != "" {
		body["effect"] = entry.Effect
	}
	if entry.Rule != "" {
		body["rule"] = entry.Rule
	}
	if len(body) > 0 {
		record.Object = auditutil.EncodeBoundedJSON(body, 64*1024)
	}
	if entry.Reason != "" {
		record.Detail = auditutil.SanitizeString(entry.Reason)
	}
	return []*object.Record{record}
}

func openAgentLlmRecord(entry openAgentAuditEntry, cursor *openAgentCursor, claim *openAgentClaim, when time.Time) []*object.Record {
	record := openAgentBaseRecord(cursor, claim, when)
	record.EventType = "llm"
	record.Action = openAgentActionOr(entry.Action, "call")
	record.Outcome = openAgentOutcome(entry.Outcome, entry.Effect)
	record.Model = entry.Model
	record.DurationMs = openAgentDuration(entry.DurationMs)
	if entry.ContentLength > 0 {
		record.Object = auditutil.EncodeBoundedJSON(map[string]any{"contentLength": entry.ContentLength}, 64*1024)
	}
	return []*object.Record{record}
}

func openAgentSessionRecord(entry openAgentAuditEntry, cursor *openAgentCursor, claim *openAgentClaim, when time.Time) []*object.Record {
	action := openAgentActionOr(entry.Action, "start")
	if action != "start" && action != "end" {
		return nil
	}
	record := openAgentBaseRecord(cursor, claim, when)
	record.EventType, record.Action = "session", action
	return []*object.Record{record}
}

func openAgentBaseRecord(cursor *openAgentCursor, claim *openAgentClaim, when time.Time) *object.Record {
	return &object.Record{
		CreatedTime: when.Format(openAgentRecordTimeFormat),
		Agent:       openAgentAgentID,
		AgentPath:   claim.Path,
		User:        claim.Owner,
		SessionKey:  cursor.SessionKey,
	}
}

func openAgentOutcome(outcome, effect string) string {
	if strings.EqualFold(effect, "deny") {
		return "denied"
	}
	switch strings.ToLower(strings.TrimSpace(outcome)) {
	case "success", "failure", "denied", "attempted":
		return strings.ToLower(outcome)
	case "":
		return "attempted"
	default:
		return strings.ToLower(outcome)
	}
}

func openAgentActionOr(action, fallback string) string {
	if trimmed := strings.ToLower(strings.TrimSpace(action)); trimmed != "" {
		return trimmed
	}
	return fallback
}

func openAgentDuration(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func openAgentTimestamp(value string) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed
	}
	return time.Now()
}
