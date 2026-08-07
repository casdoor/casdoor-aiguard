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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"xorm.io/xorm"
)

// resetRecordStore points the shared database at a fresh SQLite file under
// t.TempDir() and creates its schema, so tests do not see each other's
// records or touch a real database. It also clears the in-memory live
// session title override records still keeps (see SetSessionTitle).
func resetRecordStore(t *testing.T) {
	t.Helper()
	engine, err := xorm.NewEngine("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := engine.Sync2(new(Record), new(Event)); err != nil {
		t.Fatalf("sync test database schema: %v", err)
	}
	t.Cleanup(func() { engine.Close() })
	db = engine
	records = &recordStore{sessionTitles: map[string]string{}}
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

func TestListSessionsPrefersLiveTitleOverAgentReportedOne(t *testing.T) {
	resetRecordStore(t)

	AddRecord(&Record{
		Agent: "claude-desktop", EventType: "session", Action: "start",
		SessionKey: "s1", Title: "Stale title", CreatedTime: "2026-01-01T00:00:00.000Z",
	})
	SetSessionTitle("s1", "Live Cowork title")

	sessions := ListSessions()
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Title != "Live Cowork title" {
		t.Errorf("expected the live title override to win, got %q", sessions[0].Title)
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

func TestListRecordsFilteredByAgentEventTypeAndOutcome(t *testing.T) {
	resetRecordStore(t)

	AddRecord(&Record{Agent: "openagent", EventType: "tool", Outcome: "success", CreatedTime: "2026-01-01T00:00:00.000Z"})
	AddRecord(&Record{Agent: "openagent", EventType: "llm", Outcome: "success", CreatedTime: "2026-01-01T00:00:01.000Z"})
	AddRecord(&Record{Agent: "claude-code", EventType: "tool", Outcome: "denied", CreatedTime: "2026-01-01T00:00:02.000Z"})

	result := ListRecordsFiltered(RecordFilter{Agent: "OpenAgent"}, 0)
	if len(result) != 2 {
		t.Fatalf("expected 2 openagent records (case-insensitive), got %d: %+v", len(result), result)
	}

	result = ListRecordsFiltered(RecordFilter{EventType: "TOOL"}, 0)
	if len(result) != 2 {
		t.Fatalf("expected 2 tool records, got %d: %+v", len(result), result)
	}

	result = ListRecordsFiltered(RecordFilter{Outcome: "denied"}, 0)
	if len(result) != 1 || result[0].Agent != "claude-code" {
		t.Fatalf("expected 1 denied record, got %+v", result)
	}
}

func TestListRecordsFilteredOrdersNewestFirstAndRespectsLimit(t *testing.T) {
	resetRecordStore(t)

	for i := 0; i < 5; i++ {
		AddRecord(&Record{
			Agent: "openagent", EventType: "tool", ToolName: fmt.Sprintf("tool-%d", i),
			CreatedTime: fmt.Sprintf("2026-01-01T00:00:0%d.000Z", i),
		})
	}

	result := ListRecordsFiltered(RecordFilter{}, 2)
	if len(result) != 2 {
		t.Fatalf("expected limit to cap the result at 2, got %d", len(result))
	}
	if result[0].ToolName != "tool-4" || result[1].ToolName != "tool-3" {
		t.Fatalf("expected the 2 newest records in order, got %+v", result)
	}
}

// TestSetRecordFeedbackPersistsAcrossReopeningTheDatabase is the direct
// replacement for the old ring-buffer replay tests: a real database persists
// by construction, so a "restart" here is simply closing and reopening the
// same SQLite file, with no reload step of any kind needed for the record -
// including its corrected feedback - to still be there.
func TestSetRecordFeedbackPersistsAcrossReopeningTheDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	engine, err := xorm.NewEngine("sqlite", path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := engine.Sync2(new(Record), new(Event)); err != nil {
		t.Fatalf("sync schema: %v", err)
	}
	db = engine
	records = &recordStore{sessionTitles: map[string]string{}}

	AddRecord(&Record{
		Agent: "claude-code", EventType: "tool", Action: "call", SessionKey: "s1",
		CreatedTime: "2026-01-01T00:00:00.000Z", Resource: "fs#write", Intent: "tool.call",
		IsTriggered: true, IsAllowed: false,
	})
	before := ListRecordsFiltered(RecordFilter{}, 0)
	if len(before) != 1 {
		t.Fatalf("setup: expected 1 record, got %d", len(before))
	}
	originalId := before[0].Id
	if _, err := SetRecordFeedback(originalId, FeedbackAllow, "alice"); err != nil {
		t.Fatalf("SetRecordFeedback: %v", err)
	}
	engine.Close()

	// Reopen the same file - a fresh process would do exactly this on
	// restart, via InitDatabase.
	reopened, err := xorm.NewEngine("sqlite", path)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer reopened.Close()
	if err := reopened.Sync2(new(Record), new(Event)); err != nil {
		t.Fatalf("sync schema on reopen: %v", err)
	}
	db = reopened

	after := ListRecordsFiltered(RecordFilter{}, 0)
	if len(after) != 1 {
		t.Fatalf("expected the record to survive reopening the database, got %d records", len(after))
	}
	if after[0].Id != originalId {
		t.Errorf("expected the same Id %q to survive, got %q", originalId, after[0].Id)
	}
	if after[0].Feedback != FeedbackAllow {
		t.Errorf("expected the corrected feedback %q to survive, got %q", FeedbackAllow, after[0].Feedback)
	}
}

func TestSetRecordFeedbackRejectsAnUncorrectableRecord(t *testing.T) {
	resetRecordStore(t)

	AddRecord(&Record{Agent: "claude-code", EventType: "tool", CreatedTime: "2026-01-01T00:00:00.000Z"})
	result := ListRecordsFiltered(RecordFilter{}, 0)
	if len(result) != 1 {
		t.Fatalf("setup: expected 1 record, got %d", len(result))
	}

	if _, err := SetRecordFeedback(result[0].Id, FeedbackAllow, "alice"); err == nil {
		t.Error("expected an error correcting a record with no Resource/Intent, got none")
	}
}

func TestSetRecordFeedbackRejectsAnUnknownId(t *testing.T) {
	resetRecordStore(t)

	if _, err := SetRecordFeedback("does-not-exist", FeedbackAllow, "alice"); err == nil {
		t.Error("expected an error correcting an unknown record Id, got none")
	}
}

// TestImportRecordLogFromImportsLegacyHistoryOnAFreshDatabase covers the
// upgrade path: a database created for the first time on a host that already
// has an old record.log must not start empty.
func TestImportRecordLogFromImportsLegacyHistoryOnAFreshDatabase(t *testing.T) {
	resetRecordStore(t)

	path := filepath.Join(t.TempDir(), "record.log")
	content := `{"id":"r1","agent":"openagent","eventType":"tool","createdTime":"2026-01-01T00:00:00.000Z"}` + "\n" +
		`{"id":"r2","agent":"openagent","eventType":"tool","createdTime":"2026-01-01T00:00:01.000Z"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := importRecordLogFrom(path); err != nil {
		t.Fatalf("importRecordLogFrom: %v", err)
	}

	result := ListRecordsFiltered(RecordFilter{}, 0)
	if len(result) != 2 {
		t.Fatalf("expected the 2 legacy records to be imported, got %d: %+v", len(result), result)
	}
}

// TestImportRecordLogFromIsIdempotent is the SQLite-backed equivalent of the
// double-restart regression this repo has already hit once: an earlier
// ring-buffer-replay design re-imported the same history - and re-appended
// it to the very file it read from - on every restart, doubling the record
// log (and every count built from it) each time. Importing into a real
// database must only ever happen once, on the first startup after the file
// exists, never again once the database has rows of its own.
func TestImportRecordLogFromIsIdempotent(t *testing.T) {
	resetRecordStore(t)

	path := filepath.Join(t.TempDir(), "record.log")
	content := `{"id":"r1","agent":"openagent","eventType":"tool","createdTime":"2026-01-01T00:00:00.000Z"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		if err := importRecordLogFrom(path); err != nil {
			t.Fatalf("importRecordLogFrom (call %d): %v", i+1, err)
		}
	}

	result := ListRecordsFiltered(RecordFilter{}, 0)
	if len(result) != 1 {
		t.Fatalf("expected exactly 1 record after importing twice, got %d: %+v", len(result), result)
	}
}

// TestImportRecordLogFromDoesNotImportOverFreshActivity covers the other
// half of "only once": a database that already has rows of its own - because
// a record has already been reported since this process started, with no
// legacy file ever imported - must not have that fresh activity clobbered or
// duplicated by an import running late.
func TestImportRecordLogFromDoesNotImportOverFreshActivity(t *testing.T) {
	resetRecordStore(t)

	AddRecord(&Record{Agent: "claude-code", EventType: "tool", CreatedTime: "2026-01-01T00:00:00.000Z"})

	path := filepath.Join(t.TempDir(), "record.log")
	content := `{"id":"legacy","agent":"openagent","eventType":"tool","createdTime":"2020-01-01T00:00:00.000Z"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := importRecordLogFrom(path); err != nil {
		t.Fatalf("importRecordLogFrom: %v", err)
	}

	result := ListRecordsFiltered(RecordFilter{}, 0)
	if len(result) != 1 {
		t.Fatalf("expected the database's own record to be untouched, not merged with the legacy file, got %d: %+v", len(result), result)
	}
}

func TestImportRecordLogFromKeepsOnlyTheCorrectedStateOfAFeedbackRecord(t *testing.T) {
	resetRecordStore(t)

	path := filepath.Join(t.TempDir(), "record.log")
	original := `{"id":"r1","agent":"claude-code","eventType":"tool","createdTime":"2026-01-01T00:00:00.000Z","resource":"fs#write","intent":"tool.call","isTriggered":true,"isAllowed":false}` + "\n"
	corrected := `{"id":"r1","agent":"claude-code","eventType":"tool","createdTime":"2026-01-01T00:00:00.000Z","resource":"fs#write","intent":"tool.call","isTriggered":true,"isAllowed":false,"feedback":"allow","feedbackBy":"alice"}` + "\n"
	if err := os.WriteFile(path, []byte(original+corrected), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := importRecordLogFrom(path); err != nil {
		t.Fatalf("importRecordLogFrom: %v", err)
	}

	result := ListRecordsFiltered(RecordFilter{}, 0)
	if len(result) != 1 {
		t.Fatalf("expected 1 record after import (not one per legacy line), got %d: %+v", len(result), result)
	}
	if result[0].Feedback != FeedbackAllow {
		t.Errorf("expected the imported record to carry its corrected feedback %q, got %q", FeedbackAllow, result[0].Feedback)
	}
}

func TestImportRecordLogFromMissingFileIsNotAnError(t *testing.T) {
	resetRecordStore(t)

	if err := importRecordLogFrom(filepath.Join(t.TempDir(), "does-not-exist.log")); err != nil {
		t.Errorf("a record log that has never been created yet should not be an error, got: %v", err)
	}
	if result := ListRecordsFiltered(RecordFilter{}, 0); len(result) != 0 {
		t.Errorf("expected no records imported, got %+v", result)
	}
}

func TestImportRecordLogFromSkipsMalformedLines(t *testing.T) {
	resetRecordStore(t)

	path := filepath.Join(t.TempDir(), "record.log")
	content := "not json\n" + `{"id":"r1","agent":"claude-code","eventType":"tool","createdTime":"2026-01-01T00:00:00.000Z"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := importRecordLogFrom(path); err != nil {
		t.Fatalf("importRecordLogFrom: %v", err)
	}
	result := ListRecordsFiltered(RecordFilter{}, 0)
	if len(result) != 1 || result[0].Id != "r1" {
		t.Errorf("expected the malformed line to be skipped and the valid one kept, got %+v", result)
	}
}

// TestImportRecordLogFromDoesNotCapHistory is the positive counterpart to
// the old ring buffer's recordRingCapacity: a real database has no artificial
// cap on how much history the Records/Sessions pages can show.
func TestImportRecordLogFromDoesNotCapHistory(t *testing.T) {
	resetRecordStore(t)

	path := filepath.Join(t.TempDir(), "record.log")
	var content string
	const total = 500
	for i := 0; i < total; i++ {
		content += fmt.Sprintf(`{"id":"r%d","agent":"openagent","eventType":"tool","createdTime":"2026-01-01T%02d:%02d:%02d.000Z"}`+"\n",
			i, (i/3600)%24, (i/60)%60, i%60)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := importRecordLogFrom(path); err != nil {
		t.Fatalf("importRecordLogFrom: %v", err)
	}
	if result := ListRecordsFiltered(RecordFilter{}, 0); len(result) != total {
		t.Fatalf("expected all %d imported records, got %d", total, len(result))
	}
}
