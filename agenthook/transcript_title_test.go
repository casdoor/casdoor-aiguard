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

package agenthook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTranscript(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write test transcript: %v", err)
	}
	return path
}

func TestReadLatestAiTitleFindsMatch(t *testing.T) {
	path := writeTranscript(t, []string{
		`{"type":"user","sessionId":"s1","message":"hello"}`,
		`{"type":"ai-title","aiTitle":"Fix the login bug","sessionId":"s1"}`,
		`{"type":"assistant","sessionId":"s1","message":"working on it"}`,
	})

	if got := readLatestAiTitle(path, "s1"); got != "Fix the login bug" {
		t.Errorf("expected %q, got %q", "Fix the login bug", got)
	}
}

func TestReadLatestAiTitleReturnsMostRecent(t *testing.T) {
	path := writeTranscript(t, []string{
		`{"type":"ai-title","aiTitle":"Draft title","sessionId":"s1"}`,
		`{"type":"assistant","sessionId":"s1","message":"..."}`,
		`{"type":"ai-title","aiTitle":"Refined title","sessionId":"s1"}`,
	})

	if got := readLatestAiTitle(path, "s1"); got != "Refined title" {
		t.Errorf("expected the latest title %q, got %q", "Refined title", got)
	}
}

func TestReadLatestAiTitleIgnoresOtherSessions(t *testing.T) {
	path := writeTranscript(t, []string{
		`{"type":"ai-title","aiTitle":"Someone else's session","sessionId":"other"}`,
	})

	if got := readLatestAiTitle(path, "s1"); got != "" {
		t.Errorf("expected no match across sessions, got %q", got)
	}
}

func TestReadLatestAiTitleNoEntries(t *testing.T) {
	path := writeTranscript(t, []string{
		`{"type":"user","sessionId":"s1","message":"hello"}`,
	})

	if got := readLatestAiTitle(path, "s1"); got != "" {
		t.Errorf("expected empty title when the transcript has none, got %q", got)
	}
}

func TestReadLatestAiTitleMissingFile(t *testing.T) {
	if got := readLatestAiTitle(filepath.Join(t.TempDir(), "missing.jsonl"), "s1"); got != "" {
		t.Errorf("expected empty title for a missing file, got %q", got)
	}
}

func TestReadLatestAiTitleRedactsCredentialLookingText(t *testing.T) {
	path := writeTranscript(t, []string{
		`{"type":"ai-title","aiTitle":"Authorization: Bearer abc123.def456.ghi789","sessionId":"s1"}`,
	})

	got := readLatestAiTitle(path, "s1")
	if strings.Contains(got, "abc123.def456.ghi789") {
		t.Errorf("expected the bearer token to be redacted, got %q", got)
	}
}

func TestReadLatestAiTitleSkipsPastTailWindow(t *testing.T) {
	lines := []string{`{"type":"ai-title","aiTitle":"Too old to see","sessionId":"s1"}`}
	// Pad the file well past titleTailBytes with content the tail read never
	// reaches, so the seek-forward window is actually exercised.
	padding := strings.Repeat("x", 100)
	for i := 0; i < (titleTailBytes/len(padding))+10; i++ {
		lines = append(lines, `{"type":"user","sessionId":"s1","message":"`+padding+`"}`)
	}
	path := writeTranscript(t, lines)

	if got := readLatestAiTitle(path, "s1"); got != "" {
		t.Errorf("expected the title before the tail window to be missed, got %q", got)
	}
}
