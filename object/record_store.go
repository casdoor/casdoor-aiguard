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
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/beego/beego/v2/core/logs"
	"github.com/casdoor/casdoor-aiguard/conf"
)

// recordStore holds the one piece of session state that is deliberately
// never persisted: a live title override. See SetSessionTitle.
type recordStore struct {
	mutex         sync.RWMutex
	sessionTitles map[string]string
}

var records = &recordStore{sessionTitles: map[string]string{}}

// SetSessionTitle updates the live title used by the Sessions page. A title is
// session metadata rather than a new audited operation, so it is kept as an
// in-memory override instead of a database write - it does not describe what
// happened, only what to call it right now, and is only ever as fresh as the
// agent last reported it. An empty title removes the override and restores
// the normal fallback.
func SetSessionTitle(sessionKey, title string) {
	title = strings.TrimSpace(title)
	if len(title) > maxRecordTitleBytes {
		const suffix = "..."
		limit := maxRecordTitleBytes - len(suffix)
		for limit > 0 && !utf8.RuneStart(title[limit]) {
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

// importRecordLogOnce imports GetRecordLogFile's history into the record
// table exactly once: only when the table is still empty, so a fresh install
// with no legacy file is a no-op, and a database that has already been
// migrated - or has simply been used since - is never touched again.
//
// Every field a line already has, including Id and CreatedTime, is trusted
// as-is and inserted directly, not through AddRecord, so a re-parsed old
// entry does not get a fresh identity or get re-appended anywhere.
//
// A record can appear more than once in the file: SetRecordFeedback used to
// append a correction rather than rewrite the original, back when the log
// was the append-only store of record (see this file's git history). This
// keeps only the last line for each Id - the corrected state - so the
// database ends up with the one row per record its schema expects.
func importRecordLogOnce() error {
	return importRecordLogFrom(conf.GetRecordLogFile())
}

func importRecordLogFrom(path string) error {
	count, err := db.Count(new(Record))
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

	byId := map[string]*Record{}
	var order []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxImportLine)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var record Record
		// A malformed line must not block every record after it - skip and
		// keep reading, the same defensiveness every agent audit parser in
		// this codebase already applies to its own input.
		if err := json.Unmarshal(line, &record); err != nil || record.Id == "" {
			continue
		}
		if _, seen := byId[record.Id]; !seen {
			order = append(order, record.Id)
		}
		byId[record.Id] = &record
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if len(order) == 0 {
		return nil
	}

	imported := make([]*Record, len(order))
	for i, id := range order {
		imported[i] = byId[id]
	}
	if _, err := db.Insert(imported); err != nil {
		return err
	}
	logs.Info("imported %d record(s) from the legacy record log", len(imported))
	return nil
}

// AddRecord stores one behaviour record reported by a patched agent.
func AddRecord(r *Record) {
	r.normalize()
	if _, err := db.Insert(r); err != nil {
		logs.Warn("failed to insert record: %v", err)
	}
}

// SetRecordFeedback records one operator's correction of a verdict and
// returns the corrected record.
//
// Only a record the enforcer ruled on can be corrected, because only those
// carry the Casbin triple a self-learned rule would be written about.
func SetRecordFeedback(id string, feedback string, by string) (*Record, error) {
	if feedback != FeedbackNone && feedback != FeedbackAllow && feedback != FeedbackDeny {
		return nil, fmt.Errorf("unknown feedback %q, expected %q or %q", feedback, FeedbackAllow, FeedbackDeny)
	}

	var record Record
	has, err := db.Where("id = ?", id).Get(&record)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, fmt.Errorf("the record %q was not found", id)
	}
	if !record.IsCorrectable() {
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
	if _, err := db.ID(record.Id).Cols("feedback", "feedback_by", "feedback_time").Update(&record); err != nil {
		return nil, err
	}
	return &record, nil
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

// RecordFilter narrows the Records page. Every field is optional and
// matching is case-insensitive.
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
	session := db.NewSession()
	defer session.Close()

	// Ordered by insertion (rowid), newest first, and limited before the time
	// sort below - the same window a bounded ring buffer used to give: up to
	// `limit` of the most recently reported matching records, not the top
	// `limit` across all of history by timestamp.
	session = session.OrderBy("rowid DESC")
	if filter.Agent != "" {
		session = session.Where("LOWER(agent) = LOWER(?)", filter.Agent)
	}
	if filter.EventType != "" {
		session = session.Where("LOWER(event_type) = LOWER(?)", filter.EventType)
	}
	if filter.Outcome != "" {
		session = session.Where("LOWER(outcome) = LOWER(?)", filter.Outcome)
	}
	if filter.SessionKey != "" {
		session = session.Where("LOWER(session_key) = LOWER(?)", filter.SessionKey)
	}
	if limit > 0 {
		session = session.Limit(limit)
	}

	result := []*Record{}
	if err := session.Find(&result); err != nil {
		logs.Warn("failed to list records: %v", err)
		return []*Record{}
	}

	// A stable sort so records sharing a timestamp, or carrying one this
	// process cannot parse, keep the insertion order the query above already
	// gave them rather than being shuffled again. Agents report each event on
	// its own connection, so several events from one burst routinely arrive
	// (and are inserted) out of the order they actually happened in; sorting
	// on the agent's own timestamp is what keeps the Records page in
	// sequence.
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

// ListSessions groups every record by SessionKey and summarizes each group
// into one row, newest session first. Records without a SessionKey -
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
