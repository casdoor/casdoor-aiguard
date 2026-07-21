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
	"strings"
	"sync"

	"github.com/beego/beego/v2/core/logs"
	"github.com/casdoor/casdoor-aiguard/conf"
)

// recordRingCapacity bounds the in-memory record buffer behind the Records page.
// Full history lives in the append-only record log file, not in memory - the
// same split the event store makes.
const recordRingCapacity = 5000

type recordStore struct {
	mutex   sync.RWMutex
	records []*Record
	head    int
	size    int

	logFile *os.File
}

var records = newRecordStore()

func newRecordStore() *recordStore {
	return &recordStore{records: make([]*Record, recordRingCapacity)}
}

// InitRecordLog opens the record log file for appending. Call once at startup.
func InitRecordLog() error {
	path := conf.GetRecordLogFile()
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	records.mutex.Lock()
	records.logFile = f
	records.mutex.Unlock()
	return nil
}

// AddRecord stores one behaviour record reported by a patched agent: into the
// ring buffer for the Records page, and as one JSON line in the record log.
func AddRecord(r *Record) {
	r.normalize()

	records.mutex.Lock()
	records.records[records.head] = r
	records.head = (records.head + 1) % recordRingCapacity
	if records.size < recordRingCapacity {
		records.size++
	}
	f := records.logFile
	records.mutex.Unlock()

	if f != nil {
		line, err := json.Marshal(r)
		if err == nil {
			if _, err := f.Write(append(line, '\n')); err != nil {
				logs.Warn("failed to write record log: %v", err)
			}
		}
	}
}

// ListRecords returns up to `limit` most recent records, newest first. An empty
// agent means every agent; limit <= 0 means all. The result is never nil, so the
// API reports an empty list rather than null.
func ListRecords(agent string, limit int) []*Record {
	records.mutex.RLock()
	defer records.mutex.RUnlock()

	if limit <= 0 {
		limit = records.size
	}

	result := make([]*Record, 0, min(limit, records.size))
	for i := 0; i < records.size && len(result) < limit; i++ {
		index := (records.head - 1 - i + recordRingCapacity) % recordRingCapacity
		record := records.records[index]
		if agent != "" && !strings.EqualFold(record.Agent, agent) {
			continue
		}
		result = append(result, record)
	}
	return result
}
