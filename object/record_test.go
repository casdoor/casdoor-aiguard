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
	"strings"
	"testing"
	"unicode/utf8"
)

// TestNormalizeTruncatesObjectOnARuneBoundary is a regression test: before
// OpenAgent's "message" records carried real conversation text, Object never
// held enough bytes to hit maxRecordObjectBytes, so a byte-exact cut never
// had a multi-byte character to land on. A long, mostly-multi-byte message
// (every character is 3 bytes in UTF-8) makes that likely, and a cut that
// lands mid-rune leaves a dangling partial sequence - not a crash, but
// json.Marshal replaces it with U+FFFD, which the UI would show as garbled
// text at the end of an otherwise readable truncation.
func TestNormalizeTruncatesObjectOnARuneBoundary(t *testing.T) {
	// Repeats a 3-byte rune enough times to comfortably clear
	// maxRecordObjectBytes, so the cut point is very unlikely to happen to
	// land on a boundary by chance alone.
	text := strings.Repeat("现", maxRecordObjectBytes)
	record := &Record{Agent: "openagent", EventType: "message", Action: "assistant", Object: text}
	record.normalize()

	if !utf8.ValidString(record.Object) {
		t.Fatalf("truncated Object is not valid UTF-8: %q", record.Object)
	}
	if strings.ContainsRune(record.Object, utf8.RuneError) {
		t.Errorf("truncated Object contains the UTF-8 replacement character, meaning a rune was split: %q", record.Object)
	}
	if !strings.HasSuffix(record.Object, "\n...[truncated]") {
		t.Errorf("expected the truncation suffix, got a string ending %q", record.Object[len(record.Object)-20:])
	}

	// The whole point: round-tripping through the same encoding the record
	// log and the API responses actually use must not introduce U+FFFD.
	encoded, err := json.Marshal(record.Object)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if strings.Contains(string(encoded), `�`) {
		t.Errorf("JSON-encoded Object contains the replacement character escape: %s", encoded)
	}
}

// TestNormalizeTruncatesTitleOnARuneBoundary covers the same bug in Title's
// identical byte-slice truncation, one field down in normalize.
func TestNormalizeTruncatesTitleOnARuneBoundary(t *testing.T) {
	title := strings.Repeat("标", maxRecordTitleBytes)
	record := &Record{Agent: "openagent", EventType: "session", Action: "title", Title: title}
	record.normalize()

	if !utf8.ValidString(record.Title) {
		t.Fatalf("truncated Title is not valid UTF-8: %q", record.Title)
	}
	if strings.ContainsRune(record.Title, utf8.RuneError) {
		t.Errorf("truncated Title contains the UTF-8 replacement character, meaning a rune was split: %q", record.Title)
	}
}

func TestTruncateUTF8(t *testing.T) {
	tests := []struct {
		name      string
		s         string
		maxBytes  int
		suffix    string
		truncated bool // whether the suffix should appear at all
	}{
		{"well under the limit, left untouched", "hello", 100, "...", false},
		{"multi-byte rune landing exactly on the cut", "现在几点", 6, "...", true},
		{"multi-byte rune requiring a backoff", "现在几点", 7, "...", true},
		{"suffix longer than maxBytes", "现在几点", 1, "...[truncated]", true},
		{"empty string", "", 10, "...", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateUTF8(tt.s, tt.maxBytes, tt.suffix)
			if !utf8.ValidString(got) {
				t.Errorf("truncateUTF8(%q, %d, %q) = %q, not valid UTF-8", tt.s, tt.maxBytes, tt.suffix, got)
			}
			if tt.truncated && !strings.HasSuffix(got, tt.suffix) {
				t.Errorf("truncateUTF8(%q, %d, %q) = %q, missing the suffix", tt.s, tt.maxBytes, tt.suffix, got)
			}
			if !tt.truncated && got != tt.s {
				t.Errorf("truncateUTF8(%q, %d, %q) = %q, expected the input back unchanged", tt.s, tt.maxBytes, tt.suffix, got)
			}
		})
	}
}
