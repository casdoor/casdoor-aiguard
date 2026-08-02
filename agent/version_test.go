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
	"os"
	"path/filepath"
	"testing"
)

func TestLdflagsVersion(t *testing.T) {
	const versionVar = "github.com/the-open-agent/openagent/internal/cli.Version"
	tests := []struct {
		name    string
		ldflags string
		want    string
	}{
		{"two-field -X", "-s -w -X " + versionVar + "=2.85.0 -X other.Commit=abc", "2.85.0"},
		{"joined -X=", "-s -w -X=" + versionVar + "=2.85.0", "2.85.0"},
		{"leading v stripped", "-X " + versionVar + "=v2.85.0", "2.85.0"},
		{"describe suffix collapsed", "-X " + versionVar + "=v1.2.3-5-gabcdef", "1.2.3"},
		{"dirty describe collapsed", "-X " + versionVar + "=v2.85.0-1-gd052e68c-dirty", "2.85.0"},
		{"dev is dropped", "-X " + versionVar + "=dev", ""},
		{"var absent", "-s -w -X other.Var=1.0.0", ""},
	}
	for _, test := range tests {
		if got := ldflagsVersion(test.ldflags, versionVar); got != test.want {
			t.Errorf("%s: ldflagsVersion(%q) = %q, want %q", test.name, test.ldflags, got, test.want)
		}
	}
}

func TestCleanReleaseVersion(t *testing.T) {
	tests := map[string]string{
		"v2.85.0": "2.85.0",
		"(devel)": "",
		"":        "",
		// A pseudo-version and a +dirty build are not releases worth showing.
		"v1.799.2-0.20260723171922-4f0e2ec7bcaa+dirty": "",
		"v0.0.0-20260723171922-4f0e2ec7bcaa":           "",
		"v2.85.0+incompatible":                         "",
	}
	for input, want := range tests {
		if got := cleanReleaseVersion(input); got != want {
			t.Errorf("cleanReleaseVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestExecutableVersionFile(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "openagent")
	if err := os.WriteFile(binary, []byte("not a real binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	// No version file next to the binary yet.
	if got := executableVersionFile(binary, "version"); got != "" {
		t.Errorf("missing file should yield empty, got %q", got)
	}

	// First line is the version; trailing content is ignored.
	if err := os.WriteFile(filepath.Join(dir, "version"), []byte("2.85.0\nignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := executableVersionFile(binary, "version"); got != "2.85.0" {
		t.Errorf("got %q, want %q", got, "2.85.0")
	}

	// A "dev" placeholder is treated as no version.
	if err := os.WriteFile(filepath.Join(dir, "version"), []byte("dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := executableVersionFile(binary, "version"); got != "" {
		t.Errorf("dev placeholder should yield empty, got %q", got)
	}
}
