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

package object

import (
	"os"
	"path/filepath"
	"testing"
)

// resetRecordStore gives each test its own empty ring buffer, without a log
// file, so tests do not see each other's records or touch disk.
func resetRecordStore(t *testing.T) {
	t.Helper()
	records = newRecordStore()
}

func TestInitRecordLogOpensAtOwnerOnlyPermissions(t *testing.T) {
	resetRecordStore(t)
	path := filepath.Join(t.TempDir(), "record.log")

	if err := initRecordLogAt(path); err != nil {
		t.Fatalf("initRecordLogAt: %v", err)
	}
	t.Cleanup(func() {
		records.mutex.Lock()
		if records.logFile != nil {
			records.logFile.Close()
		}
		records.mutex.Unlock()
	})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat record log: %v", err)
	}
	// This file now holds real conversation text for an OpenAgent "message"
	// record (see Record.Object's comment), so it must not be readable by any
	// other local account.
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("record log permissions = %o, want 0600", got)
	}
}

// TestInitRecordLogTightensAnExistingFilesPermissions covers the upgrade
// path: a record log created by an older aiguard (0o644, or whatever an
// operator's umask produced) must not keep that looser mode forever just
// because os.OpenFile's mode argument only applies at creation.
func TestInitRecordLogTightensAnExistingFilesPermissions(t *testing.T) {
	resetRecordStore(t)
	path := filepath.Join(t.TempDir(), "record.log")
	if err := os.WriteFile(path, []byte(`{"id":"r1"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := initRecordLogAt(path); err != nil {
		t.Fatalf("initRecordLogAt: %v", err)
	}
	t.Cleanup(func() {
		records.mutex.Lock()
		if records.logFile != nil {
			records.logFile.Close()
		}
		records.mutex.Unlock()
	})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat record log: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("record log permissions = %o, want 0600 even though the file pre-existed at a looser mode", got)
	}
}

func TestListSessionsGroupsAndSummarizes(t *testing.T) {
	resetRecordStore(t)

	AddRecord(&Record{
		Agent: "claude-code", EventType: "session", Action: "start",
		SessionKey: "s1", CreatedTime: "2026-01-01T00:00:00.000Z",
	})
	AddRecord(&Record{
		Agent: "claude-code", EventType: "tool", Action: "call", Outcome: "success",
		SessionKey: "s1", ToolName: "Bash", CreatedTime: "2026-01-01T00:00:01.000Z",
	})
	AddRecord(&Record{
		Agent: "claude-code", EventType: "tool", Action: "call", Outcome: "denied",
		SessionKey: "s1", ToolName: "WebSearch", CreatedTime: "2026-01-01T00:00:02.000Z",
		IsTriggered: true, Resource: "web#search", Intent: "tool.call", IsAllowed: false,
	})
	AddRecord(&Record{
		Agent: "claude-code", EventType: "session", Action: "start",
		SessionKey: "s2", CreatedTime: "2026-01-01T00:00:03.000Z",
	})
	// A record with no session key is not a session and must not appear.
	AddRecord(&Record{Agent: "claude-code", EventType: "session", Action: "end", CreatedTime: "2026-01-01T00:00:04.000Z"})

	sessions := ListSessions()
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d: %+v", len(sessions), sessions)
	}

	// Newest session (s2) first.
	if sessions[0].SessionKey != "s2" {
		t.Errorf("expected s2 first (newest), got %q", sessions[0].SessionKey)
	}

	s1 := sessions[1]
	if s1.SessionKey != "s1" {
		t.Fatalf("expected s1 second, got %q", s1.SessionKey)
	}
	if s1.RecordCount != 3 {
		t.Errorf("expected 3 records in s1, got %d", s1.RecordCount)
	}
	if s1.BlockedCount != 1 {
		t.Errorf("expected 1 blocked record in s1, got %d", s1.BlockedCount)
	}
	if s1.Title != "Bash" {
		t.Errorf("expected title %q (first tool used), got %q", "Bash", s1.Title)
	}
	if s1.FirstTime != "2026-01-01T00:00:00.000Z" {
		t.Errorf("expected first time to be the oldest record's time, got %q", s1.FirstTime)
	}
	if s1.LastTime != "2026-01-01T00:00:02.000Z" {
		t.Errorf("expected last time to be the newest record's time, got %q", s1.LastTime)
	}
}

func TestListSessionsPrefersAgentReportedTitle(t *testing.T) {
	resetRecordStore(t)

	AddRecord(&Record{
		Agent: "claude-code", EventType: "tool", Action: "call",
		SessionKey: "s1", ToolName: "Bash", CreatedTime: "2026-01-01T00:00:00.000Z",
	})
	AddRecord(&Record{
		Agent: "claude-code", EventType: "session", Action: "stop",
		SessionKey: "s1", Title: "Fix the login bug", CreatedTime: "2026-01-01T00:00:01.000Z",
	})

	sessions := ListSessions()
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Title != "Fix the login bug" {
		t.Errorf("expected the agent-reported title to win over the guessed one, got %q", sessions[0].Title)
	}
}

func TestListSessionsTitleFallsBackToEventWithoutTool(t *testing.T) {
	resetRecordStore(t)

	AddRecord(&Record{
		Agent: "claude-code", EventType: "session", Action: "start",
		SessionKey: "s1", CreatedTime: "2026-01-01T00:00:00.000Z",
	})

	sessions := ListSessions()
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Title != "session start" {
		t.Errorf("expected fallback title %q, got %q", "session start", sessions[0].Title)
	}
}

func TestListRecordsFilteredBySessionKey(t *testing.T) {
	resetRecordStore(t)

	AddRecord(&Record{Agent: "claude-code", EventType: "tool", SessionKey: "s1", CreatedTime: "2026-01-01T00:00:00.000Z"})
	AddRecord(&Record{Agent: "claude-code", EventType: "tool", SessionKey: "s2", CreatedTime: "2026-01-01T00:00:01.000Z"})

	result := ListRecordsFiltered(RecordFilter{SessionKey: "s1"}, 0)
	if len(result) != 1 || result[0].SessionKey != "s1" {
		t.Fatalf("expected only s1's record, got %+v", result)
	}

	// Matching is case-insensitive, like the other RecordFilter fields.
	result = ListRecordsFiltered(RecordFilter{SessionKey: "S1"}, 0)
	if len(result) != 1 {
		t.Fatalf("expected case-insensitive match, got %d results", len(result))
	}
}
