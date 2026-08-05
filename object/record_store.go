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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/beego/beego/v2/core/logs"
	"github.com/casdoor/casdoor-aiguard/conf"
)

// recordRingCapacity bounds the in-memory record buffer behind the Records page.
// Full history lives in the append-only record log file, not in memory - the
// same split the event store makes.
const recordRingCapacity = 5000

type recordStore struct {
	mutex         sync.RWMutex
	records       []*Record
	head          int
	size          int
	sessionTitles map[string]string

	logFile *os.File
}

var records = newRecordStore()

func newRecordStore() *recordStore {
	return &recordStore{
		records:       make([]*Record, recordRingCapacity),
		sessionTitles: map[string]string{},
	}
}

// SetSessionTitle updates the live title used by the Sessions page. A title is
// session metadata rather than a new audited operation, so it is kept as an
// override instead of rewriting append-only records. An empty title removes
// the override and restores the normal fallback.
func SetSessionTitle(sessionKey, title string) {
	title = strings.TrimSpace(title)
	if len(title) > maxRecordTitleBytes {
		const suffix = "..."
		limit := maxRecordTitleBytes - len(suffix)
		for !utf8.ValidString(title[:limit]) {
			limit--
		}
		title = title[:limit] + suffix
	}

	records.mutex.Lock()
	defer records.mutex.Unlock()
	if title == "" {
		delete(records.sessionTitles, sessionKey)
		return
	}
	records.sessionTitles[sessionKey] = title
}

func sessionTitlesSnapshot() map[string]string {
	records.mutex.RLock()
	defer records.mutex.RUnlock()

	result := make(map[string]string, len(records.sessionTitles))
	for sessionKey, title := range records.sessionTitles {
		result[sessionKey] = title
	}
	return result
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

// SetRecordFeedback records one operator's correction of a verdict and returns
// the corrected record. The correction is appended to the record log as a fresh
// line rather than rewriting the original: the log is an append-only audit
// trail, so "this was blocked" and "a person later said it should not have
// been" are two facts about the same operation, not one replacing the other.
//
// Only a record the enforcer ruled on can be corrected, because only those
// carry the Casbin triple a self-learned rule would be written about.
func SetRecordFeedback(id string, feedback string, by string) (*Record, error) {
	if feedback != FeedbackNone && feedback != FeedbackAllow && feedback != FeedbackDeny {
		return nil, fmt.Errorf("unknown feedback %q, expected %q or %q", feedback, FeedbackAllow, FeedbackDeny)
	}

	records.mutex.Lock()
	var record *Record
	for i := 0; i < records.size; i++ {
		index := (records.head - 1 - i + recordRingCapacity) % recordRingCapacity
		if records.records[index] != nil && records.records[index].Id == id {
			record = records.records[index]
			break
		}
	}
	if record == nil {
		records.mutex.Unlock()
		return nil, fmt.Errorf("the record %q is no longer in the buffer, it may have aged out", id)
	}
	if !record.IsCorrectable() {
		records.mutex.Unlock()
		return nil, fmt.Errorf("the record %q was only logged, not ruled on, so there is no decision to correct", id)
	}

	record.Feedback = feedback
	if feedback == FeedbackNone {
		record.FeedbackBy = ""
		record.FeedbackTime = ""
	} else {
		record.FeedbackBy = by
		record.FeedbackTime = time.Now().Format(time.RFC3339)
	}
	corrected := *record
	f := records.logFile
	records.mutex.Unlock()

	if f != nil {
		if line, err := json.Marshal(&corrected); err == nil {
			if _, err := f.Write(append(line, '\n')); err != nil {
				logs.Warn("failed to write record log: %v", err)
			}
		}
	}
	return &corrected, nil
}

// CorrectedRecords returns every record an operator has given feedback on,
// newest first. It is what the self-learned policy set is rebuilt from.
func CorrectedRecords() []*Record {
	result := []*Record{}
	for _, record := range ListRecords("", 0) {
		if record.Feedback != FeedbackNone {
			result = append(result, record)
		}
	}
	return result
}

// RecordFilter narrows the Records page without changing the append-only store.
// Every field is optional and matching is case-insensitive.
type RecordFilter struct {
	Agent      string
	EventType  string
	Outcome    string
	SessionKey string
}

// ListRecords returns up to `limit` most recent records, newest first. It is
// retained as the small compatibility wrapper used by self-learning code.
func ListRecords(agent string, limit int) []*Record {
	return ListRecordsFiltered(RecordFilter{Agent: agent}, limit)
}

// ListRecordsFiltered returns up to limit records matching filter, newest
// first. limit <= 0 means all. The result is never nil, so the API reports an
// empty list rather than null.
func ListRecordsFiltered(filter RecordFilter, limit int) []*Record {
	records.mutex.RLock()
	defer records.mutex.RUnlock()

	if limit <= 0 {
		limit = records.size
	}

	// Collect in arrival order, then sort by when the events actually happened.
	// The two differ: agents report each event on its own connection, so several
	// events from one burst race each other here and arrive shuffled. Sorting on
	// the agent's own timestamp is what keeps the audit trail in sequence.
	result := make([]*Record, 0, min(limit, records.size))
	for i := 0; i < records.size && len(result) < limit; i++ {
		index := (records.head - 1 - i + recordRingCapacity) % recordRingCapacity
		record := records.records[index]
		if filter.Agent != "" && !strings.EqualFold(record.Agent, filter.Agent) {
			continue
		}
		if filter.EventType != "" && !strings.EqualFold(record.EventType, filter.EventType) {
			continue
		}
		if filter.Outcome != "" && !strings.EqualFold(record.Outcome, filter.Outcome) {
			continue
		}
		if filter.SessionKey != "" && !strings.EqualFold(record.SessionKey, filter.SessionKey) {
			continue
		}
		result = append(result, record)
	}

	// A stable sort so records sharing a timestamp, or carrying one we cannot
	// parse, keep their arrival order rather than being shuffled again.
	sort.SliceStable(result, func(i, j int) bool {
		return recordTime(result[i]).After(recordTime(result[j]))
	})
	return result
}

// recordTime parses a record's reported timestamp. Agents format it themselves,
// so an unparseable value is possible; it sorts as the zero time, which leaves
// such records at the end rather than dropping them.
func recordTime(r *Record) time.Time {
	parsed, err := time.Parse(time.RFC3339, r.CreatedTime)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// SessionSummary is one row of the Sessions page: everything a session's
// records have in common, plus what happened during it. Title prefers the
// latest live metadata an agent reports, then a label carried by a record,
// and finally a guess based on the first tool the session used.
type SessionSummary struct {
	SessionKey   string `json:"sessionKey"`
	Agent        string `json:"agent"`
	Title        string `json:"title"`
	RecordCount  int    `json:"recordCount"`
	FirstTime    string `json:"firstTime"`
	LastTime     string `json:"lastTime"`
	BlockedCount int    `json:"blockedCount"`
}

// ListSessions groups every buffered record by SessionKey and summarizes each
// group into one row, newest session first. Records without a SessionKey -
// lifecycle events some hooks emit outside any session - are not a session
// and are skipped.
func ListSessions() []*SessionSummary {
	all := ListRecordsFiltered(RecordFilter{}, 0) // newest first
	sessionTitles := sessionTitlesSnapshot()

	order := []string{}
	groups := map[string][]*Record{}
	for _, record := range all {
		if record.SessionKey == "" {
			continue
		}
		if _, seen := groups[record.SessionKey]; !seen {
			order = append(order, record.SessionKey)
		}
		groups[record.SessionKey] = append(groups[record.SessionKey], record)
	}

	result := make([]*SessionSummary, 0, len(order))
	for _, key := range order {
		group := groups[key] // newest first, since `all` is
		newest, oldest := group[0], group[len(group)-1]
		blocked := 0
		for _, record := range group {
			if record.Correctable && !record.IsAllowed {
				blocked++
			}
		}
		result = append(result, &SessionSummary{
			SessionKey:   key,
			Agent:        newest.Agent,
			Title:        sessionTitle(group, sessionTitles[key]),
			RecordCount:  len(group),
			FirstTime:    oldest.CreatedTime,
			LastTime:     newest.CreatedTime,
			BlockedCount: blocked,
		})
	}
	return result
}

// sessionTitle prefers live session metadata, then the most recent title an
// agent reported on a record. Absent either, it uses the first tool or MCP
// target the session touched, then the oldest record's event/action.
func sessionTitle(newestFirst []*Record, liveTitle string) string {
	if liveTitle != "" {
		return liveTitle
	}
	for _, record := range newestFirst {
		if record.Title != "" {
			return record.Title
		}
	}

	for i := len(newestFirst) - 1; i >= 0; i-- {
		record := newestFirst[i]
		if record.McpServer != "" {
			if record.McpTool != "" {
				return record.McpServer + " / " + record.McpTool
			}
			return record.McpServer
		}
		if record.ToolName != "" {
			return record.ToolName
		}
	}
	oldest := newestFirst[len(newestFirst)-1]
	return oldest.EventType + " " + oldest.Action
}
