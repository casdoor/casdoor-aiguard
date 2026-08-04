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
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/casdoor/casdoor-aiguard/auditutil"
)

// titleTailBytes bounds how much of a transcript readLatestAiTitle reads: the
// most recent slice of the file, not the whole thing. A long session's
// transcript runs into the megabytes - mostly tool output this package never
// touches - so seeking near the end and scanning forward is the difference
// between one bounded read and rereading gigabytes over a session's life.
const titleTailBytes = 512 * 1024

// maxTitleRunes caps what readLatestAiTitle returns, defensively: the title
// is meant to be a short label, and this stops a malformed or unexpectedly
// large entry from becoming an oversized record field.
const maxTitleRunes = 200

// transcriptTitleEntry is the one line type this file reads out of a Claude
// Code transcript. Unmarshaling into this narrow struct is what keeps the scan
// cheap: every other line type - user and assistant messages, tool
// input/output, attachments - is skipped without allocating for the fields it
// carries, even if a future transcript format adds more of them.
type transcriptTitleEntry struct {
	Type      string `json:"type"`
	AiTitle   string `json:"aiTitle"`
	SessionId string `json:"sessionId"`
}

// readLatestAiTitle returns the most recent "ai-title" entry a Claude Code
// transcript recorded for sessionId, or "" if none is found in the tail read,
// the file cannot be read, or nothing matches. Claude Code writes this entry
// for its own session picker (what `claude --resume` lists), so what comes
// back is the short label the agent itself uses for the session.
//
// Best effort throughout: any error here must not fail the hook that
// triggered it, so every failure path returns "" rather than propagating.
func readLatestAiTitle(path, sessionId string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return ""
	}

	offset := int64(0)
	if info.Size() > titleTailBytes {
		offset = info.Size() - titleTailBytes
	}
	if _, err := file.Seek(offset, 0); err != nil {
		return ""
	}
	buf := make([]byte, info.Size()-offset)
	n, err := io.ReadFull(file, buf)
	// A concurrent writer can leave fewer bytes than Stat reported; the
	// partial read is still usable, so only a harder error gives up.
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return ""
	}
	buf = buf[:n]

	lines := strings.Split(string(buf), "\n")
	if offset > 0 {
		// The read started mid-file, so the first line is a partial one from
		// wherever the seek landed; it cannot parse as JSON and is dropped.
		lines = lines[1:]
	}

	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var entry transcriptTitleEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Type != "ai-title" || entry.AiTitle == "" {
			continue
		}
		if sessionId != "" && entry.SessionId != "" && entry.SessionId != sessionId {
			continue
		}
		return auditutil.SanitizeString(truncateRunes(entry.AiTitle, maxTitleRunes))
	}
	return ""
}

func truncateRunes(value string, max int) string {
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max]) + "..."
}
