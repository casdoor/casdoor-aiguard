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
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/beego/beego/v2/core/logs"
	"github.com/casdoor/casdoor-aiguard/conf"
)

// eventRingCapacity bounds the in-memory event buffer shown on the dashboard.
// Full history lives in the append-only audit log file, not in memory.
const eventRingCapacity = 2000

type eventStore struct {
	mutex  sync.RWMutex
	events []*Event
	head   int
	size   int

	auditFile *os.File
}

var store = newEventStore()

func newEventStore() *eventStore {
	return &eventStore{events: make([]*Event, eventRingCapacity)}
}

// InitAuditLog opens the audit log file for appending. Call once at startup.
func InitAuditLog() error {
	path := conf.GetAuditLogFile()
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	store.mutex.Lock()
	store.auditFile = f
	store.mutex.Unlock()
	return nil
}

// RecordEvent adds the event to the in-memory ring buffer for the dashboard
// and appends it as one JSON line to the audit log file.
func RecordEvent(e *Event) {
	store.mutex.Lock()
	store.events[store.head] = e
	store.head = (store.head + 1) % eventRingCapacity
	if store.size < eventRingCapacity {
		store.size++
	}
	f := store.auditFile
	store.mutex.Unlock()

	if f != nil {
		line, err := json.Marshal(e)
		if err == nil {
			if _, err := f.Write(append(line, '\n')); err != nil {
				logs.Warn("failed to write audit log: %v", err)
			}
		}
	}
}

// ListEvents returns up to `limit` most recent events, newest first. limit <= 0 means all.
func ListEvents(limit int) []*Event {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	if limit <= 0 || limit > store.size {
		limit = store.size
	}

	result := make([]*Event, 0, limit)
	for i := 0; i < limit; i++ {
		idx := (store.head - 1 - i + eventRingCapacity) % eventRingCapacity
		result = append(result, store.events[idx])
	}
	return result
}
