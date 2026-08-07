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

package agent

import (
	"strings"
	"testing"
	"testing/fstest"
)

// TestFingerprintsLoad turns the init-time panic on malformed registry data
// into a test failure, and pins the agents that ship: a file deleted or renamed
// by accident leaves a build that starts fine and is simply blind to an agent,
// which nothing else would catch.
func TestFingerprintsLoad(t *testing.T) {
	// File-name order, which is not quite ID order: "codex-cli.json" sorts
	// before "codex.json" because "-" precedes "." in ASCII.
	want := []string{
		"claude-code", "claude-desktop", "codex-cli", "codex", "cursor-agent",
		"cursor", "hermes-agent", "openagent", "openclaw", "windsurf",
	}

	loaded, err := loadFingerprints(fingerprintFS)
	if err != nil {
		t.Fatalf("loadFingerprints() failed: %v", err)
	}
	if len(loaded) != len(want) {
		t.Fatalf("loaded %d fingerprints, want %d", len(loaded), len(want))
	}
	for i, id := range want {
		if loaded[i].ID != id {
			t.Errorf("fingerprint %d is %q, want %q", i, loaded[i].ID, id)
		}
	}
}

// TestLoadFingerprintsRejects covers every way a hand-edited data file can be
// wrong. These used to be compile errors; now they must be load errors, because
// the alternative is an agent that silently stops being recognized or a rule
// that matches every process on the host.
func TestLoadFingerprintsRejects(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content string
		want    string
	}{{
		name:    "malformed json",
		file:    "fingerprints/broken.json",
		content: `{"id": "broken",`,
		want:    "unexpected EOF",
	}, {
		name:    "misspelled field",
		file:    "fingerprints/typo.json",
		content: `{"id": "typo", "displayName": "Typo", "cmdMarker": ["typo"]}`,
		want:    "unknown field",
	}, {
		name:    "id does not match file name",
		file:    "fingerprints/renamed.json",
		content: `{"id": "other", "displayName": "Other", "cmdMarkers": ["other"]}`,
		want:    "does not match the file name",
	}, {
		name:    "missing display name",
		file:    "fingerprints/nameless.json",
		content: `{"id": "nameless", "cmdMarkers": ["nameless"]}`,
		want:    "displayName is required",
	}, {
		name:    "empty cmdline marker matches everything",
		file:    "fingerprints/greedy.json",
		content: `{"id": "greedy", "displayName": "Greedy", "cmdMarkers": [""]}`,
		want:    "would match every process",
	}, {
		name:    "empty contains matches every path",
		file:    "fingerprints/greedypath.json",
		content: `{"id": "greedypath", "displayName": "Greedy Path", "extraExecRules": [{"suffix": "/x", "contains": [""]}]}`,
		want:    "would match every path",
	}, {
		name:    "rule that constrains nothing",
		file:    "fingerprints/dead.json",
		content: `{"id": "dead", "displayName": "Dead", "extraExecRules": [{}]}`,
		want:    "can never match",
	}, {
		name:    "no evidence at all",
		file:    "fingerprints/ghost.json",
		content: `{"id": "ghost", "displayName": "Ghost"}`,
		want:    "could never be found",
	}, {
		name:    "incomplete local server",
		file:    "fingerprints/server.json",
		content: `{"id": "server", "displayName": "Server", "localServer": {"ports": [1234]}}`,
		want:    "probePath and probeMarkers",
	}, {
		name:    "local server without ports",
		file:    "fingerprints/portless.json",
		content: `{"id": "portless", "displayName": "Portless", "localServer": {"probePath": "/health", "probeMarkers": ["x"]}}`,
		want:    "localServer.ports is required",
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := fstest.MapFS{test.file: &fstest.MapFile{Data: []byte(test.content)}}
			_, err := loadFingerprints(files)
			if err == nil {
				t.Fatalf("loadFingerprints() accepted %s", test.name)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("loadFingerprints() error = %q, want it to mention %q", err, test.want)
			}
		})
	}
}

// TestLoadFingerprintsRejectsEmptyRegistry guards the embed pattern itself: a
// glob that stops matching would otherwise produce an aiguard that recognizes
// no agent at all and reports no error doing it.
func TestLoadFingerprintsRejectsEmptyRegistry(t *testing.T) {
	_, err := loadFingerprints(fstest.MapFS{"other/claude-code.json": &fstest.MapFile{Data: []byte("{}")}})
	if err == nil || !strings.Contains(err.Error(), "no fingerprint files") {
		t.Errorf("loadFingerprints() on an empty registry = %v, want a no-fingerprint-files error", err)
	}
}

// TestFingerprintNotesAreDocumentationOnly pins that notes never reach a match
// rule. They exist to hold the reasoning that a JSON file cannot hold in a
// comment, and a scan that started reading them would be attributing agents by
// their documentation.
func TestFingerprintNotesAreDocumentationOnly(t *testing.T) {
	noted := Fingerprint{
		ID: "noted", DisplayName: "Noted",
		Notes: []string{"/usr/bin/node", "openclaw"},
	}
	if rules := deriveExecRules(noted); len(rules) != 0 {
		t.Errorf("deriveExecRules() on a notes-only fingerprint produced %d rules, want 0", len(rules))
	}
	if err := validateFingerprint(noted, "fingerprints/noted.json"); err == nil {
		t.Error("validateFingerprint() accepted a fingerprint whose only content is notes")
	}
}
