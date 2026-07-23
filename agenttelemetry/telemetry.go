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

// Package agenttelemetry converts agent OTLP log events into behaviour records.
// Agent-specific names stay in profiles; OTLP decoding and normalization are
// shared.
package agenttelemetry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/casdoor/casdoor-aiguard/auditutil"
	"github.com/casdoor/casdoor-aiguard/object"
)

const (
	maxPayloadBytes  = 64 * 1024
	recordTimeFormat = "2006-01-02T15:04:05.000Z07:00"
)

type eventRule struct {
	eventType string
	action    string
	outcome   string
	payload   []string
}

type profile struct {
	agent  string
	events map[string]eventRule
}

// Profiles are selected by the standard OTel service.name resource attribute.
// Another agent using the same event vocabulary needs only another entry.
var profiles = map[string]profile{
	"cowork": {
		agent: "claude-desktop",
		events: map[string]eventRule{
			"user_prompt": {
				eventType: "prompt", action: "submitted", outcome: "attempted",
				payload: []string{"event.sequence", "prompt_length", "prompt"},
			},
			"assistant_response": {
				eventType: "llm", action: "response", outcome: "success",
				payload: []string{"event.sequence", "request_id", "response_length", "response"},
			},
			"tool_result": {
				eventType: "tool", action: "call",
				payload: []string{"event.sequence", "success", "decision_type", "decision_source", "tool_result_size_bytes", "mcp_server_scope", "tool_parameters", "tool_input"},
			},
			"api_request": {
				eventType: "llm", action: "request", outcome: "success",
				payload: []string{"event.sequence", "cost_usd", "input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens", "speed"},
			},
			"api_error": {
				eventType: "llm", action: "request", outcome: "failure",
				payload: []string{"event.sequence", "status_code", "attempt", "speed"},
			},
			"tool_decision": {
				eventType: "permission", action: "decided",
				payload: []string{"event.sequence", "decision", "source"},
			},
		},
	},
}

var tokenUsageMetrics = map[string]bool{
	"input_tokens":          true,
	"output_tokens":         true,
	"cache_read_tokens":     true,
	"cache_creation_tokens": true,
}

type exportRequest struct {
	ResourceLogs []resourceLogs `json:"resourceLogs"`
}

type resourceLogs struct {
	Resource  resource    `json:"resource"`
	ScopeLogs []scopeLogs `json:"scopeLogs"`
}

type resource struct {
	Attributes []keyValue `json:"attributes"`
}

type scopeLogs struct {
	LogRecords []logRecord `json:"logRecords"`
}

type logRecord struct {
	TimeUnixNano         any        `json:"timeUnixNano"`
	ObservedTimeUnixNano any        `json:"observedTimeUnixNano"`
	Body                 otlpValue  `json:"body"`
	Attributes           []keyValue `json:"attributes"`
}

type keyValue struct {
	Key   string    `json:"key"`
	Value otlpValue `json:"value"`
}

type otlpValue struct {
	StringValue string `json:"stringValue"`
	BoolValue   *bool  `json:"boolValue"`
	IntValue    any    `json:"intValue"`
	DoubleValue any    `json:"doubleValue"`
	ArrayValue  *struct {
		Values []otlpValue `json:"values"`
	} `json:"arrayValue"`
	KvlistValue *struct {
		Values []keyValue `json:"values"`
	} `json:"kvlistValue"`
}

// Parse accepts one standard OTLP/HTTP JSON logs request. Unknown services and
// events are ignored.
func Parse(data []byte) ([]*object.Record, error) {
	var request exportRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		return nil, fmt.Errorf("invalid OTLP logs JSON: %w", err)
	}

	records := make([]*object.Record, 0)
	for _, resourceLogs := range request.ResourceLogs {
		resourceAttributes := attributeMap(resourceLogs.Resource.Attributes)
		current, ok := profiles[text(resourceAttributes["service.name"])]
		if !ok {
			continue
		}
		for _, scope := range resourceLogs.ScopeLogs {
			for _, log := range scope.LogRecords {
				attributes := make(map[string]any, len(resourceAttributes)+len(log.Attributes))
				for key, value := range resourceAttributes {
					attributes[key] = value
				}
				for _, attribute := range log.Attributes {
					attributes[attribute.Key] = attribute.Value.value()
				}
				if record := normalize(current, log, attributes); record != nil {
					records = append(records, record)
				}
			}
		}
	}
	return records, nil
}

func normalize(current profile, log logRecord, attributes map[string]any) *object.Record {
	eventName := text(attributes["event.name"])
	if eventName == "" {
		eventName = text(log.Body.value())
	}
	rule, ok := current.events[eventName]
	if !ok {
		return nil
	}

	record := &object.Record{
		Agent:       current.agent,
		CreatedTime: eventTime(log, attributes).Format(recordTimeFormat),
		SessionKey:  text(attributes["session.id"]),
		PromptId:    text(attributes["prompt.id"]),
		User:        text(attributes["user.email"]),
		Model:       text(attributes["model"]),
		DurationMs:  integer(attributes["duration_ms"]),
		ToolName:    text(attributes["tool_name"]),
		EventType:   rule.eventType,
		Action:      rule.action,
		Outcome:     rule.outcome,
	}
	if record.User == "" {
		record.User = text(attributes["user.account_id"])
	}
	if record.User == "" {
		record.User = text(attributes["user.account_uuid"])
	}

	switch eventName {
	case "tool_result":
		record.Outcome = "failure"
		if strings.EqualFold(text(attributes["success"]), "true") {
			record.Outcome = "success"
		}
		if strings.EqualFold(text(attributes["decision_type"]), "reject") {
			record.Outcome = "denied"
		}
		if encoded, ok := attributes["tool_parameters"].(string); ok {
			var parameters map[string]any
			if json.Unmarshal([]byte(encoded), &parameters) == nil {
				attributes["tool_parameters"] = parameters
			}
		}
		if parameters, ok := attributes["tool_parameters"].(map[string]any); ok {
			record.McpServer = text(parameters["mcp_server_name"])
			record.McpTool = text(parameters["mcp_tool_name"])
			if record.McpServer != "" {
				record.EventType = "mcp"
				if record.McpTool != "" {
					record.ToolName = record.McpTool
				}
			}
		}
	case "tool_decision":
		if strings.EqualFold(text(attributes["decision"]), "reject") {
			record.Outcome = "denied"
		} else {
			record.Outcome = "success"
		}
	}
	record.Detail = auditutil.SanitizeString(text(attributes["error"]))
	record.Object = payload(rule.payload, attributes, record.ToolName)
	return record
}

func payload(keys []string, attributes map[string]any, toolName string) string {
	result := make(map[string]any, len(keys))
	for _, key := range keys {
		value, ok := attributes[key]
		if !ok {
			continue
		}
		if tokenUsageMetrics[key] {
			if metric, err := strconv.ParseInt(text(value), 10, 64); err == nil && metric >= 0 {
				result[key] = metric
			}
		} else if key == "tool_input" {
			if encoded, ok := value.(string); ok {
				var decoded any
				decoder := json.NewDecoder(strings.NewReader(encoded))
				decoder.UseNumber()
				if decoder.Decode(&decoded) == nil {
					value = decoded
				}
			}
			result[key] = auditutil.SanitizeToolInput(toolName, value)
		} else {
			result[key] = auditutil.SanitizeValue(key, value)
		}
	}
	return auditutil.EncodeBoundedJSON(result, maxPayloadBytes)
}

func eventTime(log logRecord, attributes map[string]any) time.Time {
	if timestamp := text(attributes["event.timestamp"]); timestamp != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, timestamp); err == nil {
			return parsed
		}
	}
	nanoseconds := integer(log.TimeUnixNano)
	if nanoseconds == 0 {
		nanoseconds = integer(log.ObservedTimeUnixNano)
	}
	if nanoseconds != 0 {
		return time.Unix(0, nanoseconds)
	}
	return time.Now()
}

func attributeMap(attributes []keyValue) map[string]any {
	result := make(map[string]any, len(attributes))
	for _, attribute := range attributes {
		result[attribute.Key] = attribute.Value.value()
	}
	return result
}

func (value otlpValue) value() any {
	switch {
	case value.StringValue != "":
		return value.StringValue
	case value.BoolValue != nil:
		return *value.BoolValue
	case value.IntValue != nil:
		return value.IntValue
	case value.DoubleValue != nil:
		return value.DoubleValue
	case value.ArrayValue != nil:
		result := make([]any, 0, len(value.ArrayValue.Values))
		for _, item := range value.ArrayValue.Values {
			result = append(result, item.value())
		}
		return result
	case value.KvlistValue != nil:
		return attributeMap(value.KvlistValue.Values)
	default:
		return nil
	}
}

func text(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	case bool:
		return strconv.FormatBool(value)
	default:
		return ""
	}
}

func integer(value any) int64 {
	switch value := value.(type) {
	case json.Number:
		result, _ := value.Int64()
		return result
	case string:
		result, _ := strconv.ParseInt(value, 10, 64)
		return result
	case float64:
		return int64(value)
	default:
		return 0
	}
}
