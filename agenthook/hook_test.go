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

package agenthook

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeFillsTitleOnTitleRefreshEvent(t *testing.T) {
	path := writeTranscript(t, []string{
		`{"type":"ai-title","aiTitle":"Add session page","sessionId":"s1"}`,
	})

	event := map[string]any{
		"hook_event_name": "Stop",
		"session_id":      "s1",
		"transcript_path": path,
	}
	record := Normalize("claude-code", event, "/usr/local/bin/claude", time.Now())
	if record == nil {
		t.Fatal("expected a record for a known claude-code event")
	}
	if record.Title != "Add session page" {
		t.Errorf("expected title %q, got %q", "Add session page", record.Title)
	}
}

func TestNormalizeSkipsTitleLookupOnNonRefreshEvent(t *testing.T) {
	path := writeTranscript(t, []string{
		`{"type":"ai-title","aiTitle":"Add session page","sessionId":"s1"}`,
	})

	event := map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      "s1",
		"tool_name":       "Bash",
		"transcript_path": path,
	}
	record := Normalize("claude-code", event, "/usr/local/bin/claude", time.Now())
	if record == nil {
		t.Fatal("expected a record for a known claude-code event")
	}
	// PreToolUse fires on every tool call; reading the transcript that often
	// would be wasteful, so it is not one of the titleRefresh events.
	if record.Title != "" {
		t.Errorf("expected no title lookup on a tool event, got %q", record.Title)
	}
}

func TestNormalizeOmitsTranscriptPathFromStoredPayload(t *testing.T) {
	path := writeTranscript(t, []string{`{"type":"ai-title","aiTitle":"Add session page","sessionId":"s1"}`})

	event := map[string]any{
		"hook_event_name": "Stop",
		"session_id":      "s1",
		"transcript_path": path,
	}
	record := Normalize("claude-code", event, "/usr/local/bin/claude", time.Now())
	if record == nil {
		t.Fatal("expected a record for a known claude-code event")
	}
	// Reading the transcript to look up a title must not leak its path into
	// the payload aiguard stores - that stays as reachable as it always was.
	if strings.Contains(record.Object, path) {
		t.Errorf("expected transcript_path to stay out of the stored payload, got %q", record.Object)
	}
}
