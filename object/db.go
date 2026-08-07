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

	"github.com/beego/beego/v2/core/logs"
	"github.com/casdoor/casdoor-aiguard/conf"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

// maxImportLine bounds one line read back from a legacy log file during a
// one-time import: generous over Record's own maxRecordObjectBytes cap
// (64 KiB) to leave headroom for JSON escaping and the rest of a record's or
// event's fields, without letting one corrupt or hostile line make the
// importer buffer unbounded input.
const maxImportLine = 1 << 20

// db is aiguard's local SQLite database, backing both records (the Records
// and Sessions pages) and events (the dashboard's event stream). Both used
// to be an in-memory ring buffer plus an append-only file, each with no way
// to reconstruct the buffer from the file after a restart; a real database
// persists correctly by construction; see InitDatabase.
var db *xorm.Engine

// InitDatabase opens (creating if needed) the local SQLite database at
// conf.GetDatabaseFile(), creates the record and event tables, and imports
// each one's legacy append-only log exactly once - see importRecordLogOnce
// and importAuditLogOnce. Call once at startup.
func InitDatabase() error {
	path := conf.GetDatabaseFile()
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	// WAL lets reads proceed alongside the one writer SQLite allows at a time;
	// busy_timeout makes a write that arrives while another is committing
	// retry for up to 5s instead of failing immediately with SQLITE_BUSY.
	// This process has several goroutines - every agent monitor, plus the
	// interception proxy - that can each want to insert a record or event at
	// any moment.
	engine, err := xorm.NewEngine("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return err
	}
	// SQLite allows only one writer at a time regardless of how many
	// connections are open, so a larger pool would just queue most
	// connections behind whichever holds the write lock instead of avoiding
	// that contention.
	engine.SetMaxOpenConns(1)
	if err := engine.Sync2(new(Record), new(Event)); err != nil {
		engine.Close()
		return err
	}
	db = engine

	if err := importRecordLogOnce(); err != nil {
		logs.Warn("failed to import the legacy record log: %v", err)
	}
	if err := importAuditLogOnce(); err != nil {
		logs.Warn("failed to import the legacy audit log: %v", err)
	}
	return nil
}
