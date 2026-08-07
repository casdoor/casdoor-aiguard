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
	"bufio"
	"encoding/json"
	"os"

	"github.com/beego/beego/v2/core/logs"
	"github.com/casdoor/casdoor-aiguard/conf"
)

// importAuditLogOnce imports GetAuditLogFile's history into the event table
// exactly once: only when the table is still empty, matching
// importRecordLogOnce's reasoning in object/record_store.go - see its
// comment. Unlike a record, an event is never corrected after the fact, so
// there is no duplicate-Id case to dedupe here.
func importAuditLogOnce() error {
	return importAuditLogFrom(conf.GetAuditLogFile())
}

func importAuditLogFrom(path string) error {
	count, err := db.Count(new(Event))
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	var imported []*Event
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxImportLine)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var event Event
		if err := json.Unmarshal(line, &event); err != nil || event.Id == "" {
			continue
		}
		imported = append(imported, &event)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if len(imported) == 0 {
		return nil
	}

	if _, err := db.Insert(imported); err != nil {
		return err
	}
	logs.Info("imported %d event(s) from the legacy audit log", len(imported))
	return nil
}

// RecordEvent stores one intercepted, recognized (or passed-through) egress
// request, shown in the dashboard's event stream.
func RecordEvent(e *Event) {
	if _, err := db.Insert(e); err != nil {
		logs.Warn("failed to insert event: %v", err)
	}
}

// ListEvents returns up to `limit` most recent events, newest first (by
// insertion, matching when RecordEvent was called - not re-sorted by
// Timestamp, since the two are the same order in practice and this avoids a
// second sort for a field that, unlike a Record's CreatedTime, is never
// agent-supplied text this process might fail to parse). limit <= 0 means
// all.
func ListEvents(limit int) []*Event {
	session := db.NewSession()
	defer session.Close()
	session = session.OrderBy("rowid DESC")
	if limit > 0 {
		session = session.Limit(limit)
	}

	result := []*Event{}
	if err := session.Find(&result); err != nil {
		logs.Warn("failed to list events: %v", err)
		return []*Event{}
	}
	return result
}
