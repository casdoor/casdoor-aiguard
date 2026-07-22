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

// Package otelreceiver converts Claude Code's native OTLP logs into the same
// privacy-bounded behaviour records used by command hooks.
package otelreceiver

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/casdoor/casdoor-aiguard/auditutil"
	"github.com/casdoor/casdoor-aiguard/object"
	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	MaxRequestBytes  = 8 * 1024 * 1024
	maxPayloadBytes  = 64 * 1024
	recordTimeFormat = "2006-01-02T15:04:05.000Z07:00"
)

var persistedEvents = map[string]bool{
	"api_request":             true,
	"api_error":               true,
	"api_refusal":             true,
	"api_retries_exhausted":   true,
	"mcp_server_connection":   true,
	"permission_mode_changed": true,
	"internal_error":          true,
}

// Parse decodes one OTLP/HTTP JSON export batch. Unknown event types and logs
// not emitted by Claude Code are intentionally ignored.
func Parse(data []byte) ([]*object.Record, error) {
	var request collectorlogsv1.ExportLogsServiceRequest
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, &request); err != nil {
		return nil, fmt.Errorf("invalid OTLP logs JSON: %w", err)
	}

	records := []*object.Record{}
	for _, resourceLogs := range request.ResourceLogs {
		resourceAttributes := attributesMap(resourceLogs.GetResource().GetAttributes())
		serviceName := stringAttribute(resourceAttributes, "service.name", "service_name")
		if !isClaudeCodeService(serviceName) {
			continue
		}
		for _, scopeLogs := range resourceLogs.ScopeLogs {
			for _, logRecord := range scopeLogs.LogRecords {
				attributes := mergeAttributes(resourceAttributes, attributesMap(logRecord.Attributes))
				if record := normalize(logRecord, attributes); record != nil {
					records = append(records, record)
				}
			}
		}
	}
	return records, nil
}

func normalize(logRecord *logsv1.LogRecord, attributes map[string]any) *object.Record {
	eventName := stringAttribute(attributes, "event.name", "event_name")
	if eventName == "" {
		eventName = valueString(anyValue(logRecord.Body))
	}
	eventName = strings.TrimPrefix(eventName, "claude_code.")
	if !persistedEvents[eventName] {
		return nil
	}

	record := &object.Record{
		Agent:               "claude-code",
		CreatedTime:         eventTime(logRecord, attributes).Format(recordTimeFormat),
		SessionKey:          stringAttribute(attributes, "session.id", "session_id"),
		User:                stringAttribute(attributes, "user.email", "user.id", "user.account_uuid", "user_email", "user_id"),
		PromptId:            stringAttribute(attributes, "prompt.id", "prompt_id"),
		RequestId:           stringAttribute(attributes, "request.id", "request_id"),
		Sequence:            int64Attribute(attributes, "event.sequence", "event_sequence", "sequence"),
		Model:               stringAttribute(attributes, "model", "model.name", "model_name"),
		DurationMs:          int64Attribute(attributes, "duration_ms", "duration.ms", "total_retry_duration_ms"),
		InputTokens:         int64Attribute(attributes, "input_tokens", "input.tokens"),
		OutputTokens:        int64Attribute(attributes, "output_tokens", "output.tokens"),
		CacheReadTokens:     int64Attribute(attributes, "cache_read_tokens", "cache.read_tokens"),
		CacheCreationTokens: int64Attribute(attributes, "cache_creation_tokens", "cache.creation_tokens"),
		CostUsd:             float64Attribute(attributes, "cost_usd", "cost.usd"),
	}
	if record.CostUsd == 0 {
		record.CostUsd = float64(int64Attribute(attributes, "cost_usd_micros")) / 1_000_000
	}

	switch eventName {
	case "api_request":
		record.EventType, record.Action, record.Outcome = "llm", "request", "success"
	case "api_error":
		record.EventType, record.Action, record.Outcome = "llm", "request", "failure"
	case "api_refusal":
		record.EventType, record.Action, record.Outcome = "llm", "refusal", "failure"
	case "api_retries_exhausted":
		record.EventType, record.Action, record.Outcome = "llm", "retries_exhausted", "failure"
	case "mcp_server_connection":
		record.EventType = "mcp"
		record.McpServer = stringAttribute(attributes, "server_name", "mcp.server.name", "mcp_server")
		status := strings.ToLower(stringAttribute(attributes, "status", "connection_status"))
		record.Action = status
		if record.Action == "" {
			record.Action = "connection"
		}
		if strings.Contains(status, "fail") || strings.Contains(status, "error") {
			record.Outcome = "failure"
		} else {
			record.Outcome = "success"
		}
	case "permission_mode_changed":
		record.EventType, record.Action, record.Outcome = "permission", "mode_changed", "success"
	case "internal_error":
		record.EventType, record.Action, record.Outcome = "runtime", "internal_error", "failure"
	}

	record.Detail = auditutil.SanitizeString(stringAttribute(attributes, "error.message", "error", "message", "reason"))
	if eventName == "internal_error" && record.Detail == "" {
		record.Detail = strings.Trim(strings.Join([]string{
			stringAttribute(attributes, "error_name"),
			stringAttribute(attributes, "error_code"),
		}, ": "), ": ")
	}
	record.Object = auditutil.EncodeBoundedJSON(auditutil.SanitizeValue("", telemetryPayload(eventName, attributes)), maxPayloadBytes)
	return record
}

// telemetryPayload is a strict allowlist. It keeps operational context useful
// while excluding prompt, tool and assistant bodies even if a future Claude
// Code version adds them to the same log record.
func telemetryPayload(eventName string, attributes map[string]any) map[string]any {
	payload := map[string]any{"event_name": eventName}
	for _, key := range []string{
		"query_source", "terminal.type", "terminal_type", "organization.id", "organization_id",
		"speed", "effort", "client_request_id", "status", "connection_status", "transport", "transport_type",
		"server_scope", "attempt", "total_attempts", "total_retry_duration_ms", "status_code", "http.status_code",
		"old_mode", "new_mode", "from_mode", "to_mode", "trigger", "server_fallback_hop", "has_category",
		"has_explanation", "category", "error.type", "error_type", "error_name", "error_code", "is_plugin",
	} {
		if value, ok := attributes[key]; ok {
			payload[key] = value
		}
	}
	return payload
}

func isClaudeCodeService(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "claude-code" || name == "claude-code-desktop"
}

func eventTime(logRecord *logsv1.LogRecord, attributes map[string]any) time.Time {
	if text := stringAttribute(attributes, "event.timestamp", "event_timestamp"); text != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
			return parsed
		}
	}
	nanoseconds := logRecord.TimeUnixNano
	if nanoseconds == 0 {
		nanoseconds = logRecord.ObservedTimeUnixNano
	}
	if nanoseconds != 0 {
		return time.Unix(0, int64(nanoseconds))
	}
	return time.Now()
}

func mergeAttributes(base map[string]any, overlay map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(overlay))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}

func attributesMap(attributes []*commonv1.KeyValue) map[string]any {
	result := make(map[string]any, len(attributes))
	for _, attribute := range attributes {
		result[attribute.Key] = anyValue(attribute.Value)
	}
	return result
}

func anyValue(value *commonv1.AnyValue) any {
	if value == nil {
		return nil
	}
	switch typed := value.Value.(type) {
	case *commonv1.AnyValue_StringValue:
		return typed.StringValue
	case *commonv1.AnyValue_BoolValue:
		return typed.BoolValue
	case *commonv1.AnyValue_IntValue:
		return typed.IntValue
	case *commonv1.AnyValue_DoubleValue:
		return typed.DoubleValue
	case *commonv1.AnyValue_BytesValue:
		return "[binary omitted]"
	case *commonv1.AnyValue_ArrayValue:
		result := make([]any, 0, len(typed.ArrayValue.Values))
		for _, item := range typed.ArrayValue.Values {
			result = append(result, anyValue(item))
		}
		return result
	case *commonv1.AnyValue_KvlistValue:
		return attributesMap(typed.KvlistValue.Values)
	default:
		return nil
	}
}

func stringAttribute(attributes map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := attributes[key]; ok {
			if text := valueString(value); text != "" {
				return text
			}
		}
	}
	return ""
}

func valueString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return ""
	}
}

func int64Attribute(attributes map[string]any, keys ...string) int64 {
	for _, key := range keys {
		switch value := attributes[key].(type) {
		case int64:
			return value
		case float64:
			return int64(value)
		case string:
			parsed, _ := strconv.ParseInt(value, 10, 64)
			if parsed != 0 {
				return parsed
			}
		}
	}
	return 0
}

func float64Attribute(attributes map[string]any, keys ...string) float64 {
	for _, key := range keys {
		switch value := attributes[key].(type) {
		case float64:
			return value
		case int64:
			return float64(value)
		case string:
			parsed, _ := strconv.ParseFloat(value, 64)
			if parsed != 0 {
				return parsed
			}
		}
	}
	return 0
}
