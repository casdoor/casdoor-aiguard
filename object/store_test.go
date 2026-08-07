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

	"github.com/casdoor/casdoor-aiguard/recognizers"
)

func TestRecordEventAndListEventsOrdersNewestFirst(t *testing.T) {
	resetRecordStore(t)

	RecordEvent(NewEvent("api.example.com", 1, "openagent", "llm", &recognizers.Intent{Category: "llm.chat"}, ActionAllow, ""))
	RecordEvent(NewEvent("api.other.com", 2, "openagent", "payment", &recognizers.Intent{Category: "payment"}, ActionDeny, "blocked"))

	result := ListEvents(0)
	if len(result) != 2 {
		t.Fatalf("expected 2 events, got %d: %+v", len(result), result)
	}
	if result[0].Destination != "api.other.com" {
		t.Errorf("expected the most recently recorded event first, got %+v", result[0])
	}
	if result[0].Intent == nil || result[0].Intent.Category != "payment" {
		t.Errorf("expected the Intent to round-trip through the database, got %+v", result[0].Intent)
	}
}

func TestListEventsRespectsLimit(t *testing.T) {
	resetRecordStore(t)

	for i := 0; i < 5; i++ {
		RecordEvent(NewEvent(fmt.Sprintf("host-%d.example.com", i), 0, "", "", nil, ActionAllow, ""))
	}

	result := ListEvents(2)
	if len(result) != 2 {
		t.Fatalf("expected limit to cap the result at 2, got %d", len(result))
	}
	if result[0].Destination != "host-4.example.com" || result[1].Destination != "host-3.example.com" {
		t.Fatalf("expected the 2 most recently recorded events in order, got %+v", result)
	}
}

func TestListEventsHandlesANilIntent(t *testing.T) {
	resetRecordStore(t)

	RecordEvent(NewEvent("api.example.com", 0, "", "", nil, ActionAllow, ""))

	result := ListEvents(0)
	if len(result) != 1 || result[0].Intent != nil {
		t.Fatalf("expected 1 event with a nil Intent, got %+v", result)
	}
}

func TestImportAuditLogFromImportsLegacyHistoryOnAFreshDatabase(t *testing.T) {
	resetRecordStore(t)

	path := filepath.Join(t.TempDir(), "audit.log")
	content := `{"id":"e1","destination":"a.example.com","timestamp":"2026-01-01T00:00:00Z"}` + "\n" +
		`{"id":"e2","destination":"b.example.com","timestamp":"2026-01-01T00:00:01Z"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := importAuditLogFrom(path); err != nil {
		t.Fatalf("importAuditLogFrom: %v", err)
	}
	if result := ListEvents(0); len(result) != 2 {
		t.Fatalf("expected the 2 legacy events to be imported, got %d: %+v", len(result), result)
	}
}

// TestImportAuditLogFromIsIdempotent mirrors
// TestImportRecordLogFromIsIdempotent in record_store_test.go: importing must
// only ever populate an empty table, never run again once the database has
// rows, or repeated restarts would duplicate history.
func TestImportAuditLogFromIsIdempotent(t *testing.T) {
	resetRecordStore(t)

	path := filepath.Join(t.TempDir(), "audit.log")
	content := `{"id":"e1","destination":"a.example.com","timestamp":"2026-01-01T00:00:00Z"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		if err := importAuditLogFrom(path); err != nil {
			t.Fatalf("importAuditLogFrom (call %d): %v", i+1, err)
		}
	}

	if result := ListEvents(0); len(result) != 1 {
		t.Fatalf("expected exactly 1 event after importing twice, got %d: %+v", len(result), result)
	}
}

func TestImportAuditLogFromMissingFileIsNotAnError(t *testing.T) {
	resetRecordStore(t)

	if err := importAuditLogFrom(filepath.Join(t.TempDir(), "does-not-exist.log")); err != nil {
		t.Errorf("an audit log that has never been created yet should not be an error, got: %v", err)
	}
}

func TestImportAuditLogFromSkipsMalformedLines(t *testing.T) {
	resetRecordStore(t)

	path := filepath.Join(t.TempDir(), "audit.log")
	content := "not json\n" + `{"id":"e1","destination":"a.example.com","timestamp":"2026-01-01T00:00:00Z"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := importAuditLogFrom(path); err != nil {
		t.Fatalf("importAuditLogFrom: %v", err)
	}
	result := ListEvents(0)
	if len(result) != 1 || result[0].Id != "e1" {
		t.Errorf("expected the malformed line to be skipped and the valid one kept, got %+v", result)
	}
}
