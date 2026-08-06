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

import "testing"

func TestParseOpenAgentAuditLineUserMessage(t *testing.T) {
	cursor := &openAgentCursor{}
	claim := &openAgentClaim{Path: "/opt/openagent/openagent", Owner: "joy"}
	line := []byte(`{"sessionId":"chat_4jas48","type":"message","role":"user","text":"现在几点？"}`)

	records := parseOpenAgentAuditLine(line, cursor, claim)
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	record := records[0]
	if record.EventType != "message" || record.Action != "user" {
		t.Errorf("EventType/Action = %q/%q, want message/user", record.EventType, record.Action)
	}
	if record.Object != "现在几点？" {
		t.Errorf("Object = %q, want the message text", record.Object)
	}
	if record.SessionKey != "chat_4jas48" {
		t.Errorf("SessionKey = %q, want chat_4jas48", record.SessionKey)
	}
}

func TestParseOpenAgentAuditLineAssistantMessage(t *testing.T) {
	cursor := &openAgentCursor{}
	claim := &openAgentClaim{Path: "/opt/openagent/openagent", Owner: "joy"}
	line := []byte(`{"sessionId":"chat_4jas48","type":"message","role":"assistant","text":"现在是 UTC 12:00。"}`)

	records := parseOpenAgentAuditLine(line, cursor, claim)
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	record := records[0]
	if record.EventType != "message" || record.Action != "assistant" {
		t.Errorf("EventType/Action = %q/%q, want message/assistant", record.EventType, record.Action)
	}
	if record.Object != "现在是 UTC 12:00。" {
		t.Errorf("Object = %q, want the message text", record.Object)
	}
}

func TestParseOpenAgentAuditLineMessageDropsEmptyText(t *testing.T) {
	cursor := &openAgentCursor{}
	claim := &openAgentClaim{Path: "/opt/openagent/openagent", Owner: "joy"}
	line := []byte(`{"sessionId":"chat_4jas48","type":"message","role":"user","text":""}`)

	if records := parseOpenAgentAuditLine(line, cursor, claim); records != nil {
		t.Errorf("expected no record for empty text, got %+v", records)
	}
}

func TestParseOpenAgentAuditLineMessageDropsUnknownRole(t *testing.T) {
	cursor := &openAgentCursor{}
	claim := &openAgentClaim{Path: "/opt/openagent/openagent", Owner: "joy"}
	line := []byte(`{"sessionId":"chat_4jas48","type":"message","role":"system","text":"ignored"}`)

	if records := parseOpenAgentAuditLine(line, cursor, claim); records != nil {
		t.Errorf("expected no record for an unrecognized role, got %+v", records)
	}
}

func TestParseOpenAgentAuditLineMessageRedactsCredentialLookingText(t *testing.T) {
	cursor := &openAgentCursor{}
	claim := &openAgentClaim{Path: "/opt/openagent/openagent", Owner: "joy"}
	line := []byte(`{"sessionId":"chat_4jas48","type":"message","role":"user","text":"Authorization: Bearer abc123.def456.ghi789"}`)

	records := parseOpenAgentAuditLine(line, cursor, claim)
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Object == "" {
		t.Fatal("expected a record with redacted text, got empty Object")
	}
	if contains := records[0].Object; contains == "Authorization: Bearer abc123.def456.ghi789" {
		t.Errorf("expected the bearer token to be redacted, got unredacted text: %q", contains)
	}
}
func TestParseOpenAgentAuditLineChatTitle(t *testing.T) {
	cursor := &openAgentCursor{}
	claim := &openAgentClaim{Path: "/opt/openagent/openagent", Owner: "joy"}
	line := []byte(`{"timestamp":"2026-08-04T18:14:10.431Z","sessionId":"chat_7eslml","type":"chat_title","title":"北京天气查询概况"}`)

	records := parseOpenAgentAuditLine(line, cursor, claim)
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	record := records[0]
	if record.Title != "北京天气查询概况" {
		t.Errorf("Title = %q, want the chat's title", record.Title)
	}
	if record.EventType != "session" || record.Action != "title" {
		t.Errorf("EventType/Action = %q/%q, want session/title", record.EventType, record.Action)
	}
	if record.SessionKey != "chat_7eslml" {
		t.Errorf("SessionKey = %q, want chat_7eslml", record.SessionKey)
	}
	// A title event carries no conversation content, only the label - it must
	// not leave the object payload it usually would for a tool_call.
	if record.Object != "" {
		t.Errorf("Object = %q, want empty - a title event has no payload to carry", record.Object)
	}
}

func TestParseOpenAgentAuditLineChatTitleUpdatesCursorSession(t *testing.T) {
	cursor := &openAgentCursor{}
	claim := &openAgentClaim{Path: "/opt/openagent/openagent", Owner: "joy"}
	line := []byte(`{"sessionId":"chat_7eslml","type":"chat_title","title":"北京天气查询概况"}`)

	parseOpenAgentAuditLine(line, cursor, claim)
	if cursor.SessionKey != "chat_7eslml" {
		t.Errorf("cursor.SessionKey = %q, want chat_7eslml", cursor.SessionKey)
	}
}

func TestParseOpenAgentAuditLineChatTitleEmptyIsDropped(t *testing.T) {
	cursor := &openAgentCursor{}
	claim := &openAgentClaim{Path: "/opt/openagent/openagent", Owner: "joy"}
	line := []byte(`{"sessionId":"chat_7eslml","type":"chat_title","title":""}`)

	if records := parseOpenAgentAuditLine(line, cursor, claim); records != nil {
		t.Errorf("expected no record for an empty title, got %+v", records)
	}
}
