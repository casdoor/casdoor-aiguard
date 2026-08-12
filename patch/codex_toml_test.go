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

package patch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// relayEdit is what codexLLMSwitcher.ApplyProvider builds, spelled out here so
// the editor can be exercised without going near the filesystem.
func relayEdit(baseURL, model string) codexTOMLEdit {
	return codexTOMLEdit{
		Root: []codexTOMLSetting{
			{Key: "model_provider", Value: `"aiguard"`},
			{Key: "model", Value: `"` + model + `"`},
		},
		Provider: []codexTOMLSetting{
			{Key: "name", Value: `"AIGuard"`},
			{Key: "base_url", Value: `"` + baseURL + `"`},
			{Key: "wire_api", Value: `"responses"`},
			{Key: "requires_openai_auth", Value: "true"},
		},
	}
}

// clearEdit is what codexLLMSwitcher.ClearProvider builds for a target it owns.
var clearEdit = codexTOMLEdit{
	Root:              []codexTOMLSetting{{Key: "model_provider"}, {Key: "model"}},
	DropProviderTable: true,
}

func assertTOML(t *testing.T, got []byte, want string) {
	t.Helper()
	if string(got) != want {
		t.Errorf("edited config.toml:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestEditCodexTOMLKeepsEverythingItDoesNotOwn is the reason this editor
// exists at all. Marshaling the parsed document back out would drop every
// comment, reflow every table and reorder every key; config.toml is
// hand-maintained, so a switch has to read as a three-line diff.
func TestEditCodexTOMLKeepsEverythingItDoesNotOwn(t *testing.T) {
	original := `# Codex settings, hand-maintained.
model = "gpt-5"   # the model I usually use
approval_policy = "on-request"

[sandbox_workspace_write]
network_access = true

# Docs lookup, do not remove.
[mcp_servers.docs]
command = "npx"
args = [
  "-y",
  "@my/docs-server",
]

[[profiles]]
name = "review"
`

	updated, err := editCodexTOML([]byte(original), relayEdit("https://api.example.com/v1", "gpt-5-codex"))
	if err != nil {
		t.Fatalf("editCodexTOML: %v", err)
	}

	assertTOML(t, updated, `# Codex settings, hand-maintained.
model = "gpt-5-codex"   # the model I usually use
approval_policy = "on-request"
model_provider = "aiguard"

[sandbox_workspace_write]
network_access = true

# Docs lookup, do not remove.
[mcp_servers.docs]
command = "npx"
args = [
  "-y",
  "@my/docs-server",
]

[[profiles]]
name = "review"

[model_providers.aiguard]
name = "AIGuard"
base_url = "https://api.example.com/v1"
wire_api = "responses"
requires_openai_auth = true
`)
}

// TestEditCodexTOMLClearRemovesOnlyItsOwnLines is the other half: switching
// back to the default takes aiguard's table and active-model keys out and
// leaves the file otherwise as it was found.
func TestEditCodexTOMLClearRemovesOnlyItsOwnLines(t *testing.T) {
	original := `# Codex settings, hand-maintained.
model_provider = "aiguard"
model = "gpt-5-codex"
approval_policy = "on-request"

[mcp_servers.docs]
command = "npx"

[model_providers.aiguard]
name = "AIGuard"
base_url = "https://api.example.com/v1"
wire_api = "responses"
requires_openai_auth = true
`

	updated, err := editCodexTOML([]byte(original), clearEdit)
	if err != nil {
		t.Fatalf("editCodexTOML: %v", err)
	}

	assertTOML(t, updated, `# Codex settings, hand-maintained.
approval_policy = "on-request"

[mcp_servers.docs]
command = "npx"
`)
}

// TestEditCodexTOMLUpdatesAnExistingProviderTableInPlace covers a second
// switch: the table is already there, so its keys are rewritten where they sit
// and only the ones that were never written get appended.
func TestEditCodexTOMLUpdatesAnExistingProviderTableInPlace(t *testing.T) {
	original := `[model_providers.aiguard]
# written by an older aiguard
base_url = "https://old.example.com"   # keep this comment
wire_api = "responses"

[mcp_servers.docs]
command = "npx"
`

	updated, err := editCodexTOML([]byte(original), relayEdit("https://new.example.com/v1", "gpt-5-codex"))
	if err != nil {
		t.Fatalf("editCodexTOML: %v", err)
	}

	assertTOML(t, updated, `model_provider = "aiguard"
model = "gpt-5-codex"

[model_providers.aiguard]
name = "AIGuard"
requires_openai_auth = true
# written by an older aiguard
base_url = "https://new.example.com/v1"   # keep this comment
wire_api = "responses"

[mcp_servers.docs]
command = "npx"
`)
}

// TestEditCodexTOMLDoesNotReadInsideAMultiLineValue pins the line walker's one
// real hazard: a nested array's "[1, 2]," line looks exactly like a table
// header, and misreading it would attribute every following assignment to a
// table that does not exist - and then append a duplicate root key.
func TestEditCodexTOMLDoesNotReadInsideAMultiLineValue(t *testing.T) {
	original := `matrix = [
  [1, 2],
  [3, 4],
]
notes = """
[model_providers.aiguard]
model = "not really a setting"
"""
model = "gpt-5"
`

	updated, err := editCodexTOML([]byte(original), relayEdit("https://api.example.com/v1", "gpt-5-codex"))
	if err != nil {
		t.Fatalf("editCodexTOML: %v", err)
	}

	assertTOML(t, updated, `matrix = [
  [1, 2],
  [3, 4],
]
notes = """
[model_providers.aiguard]
model = "not really a setting"
"""
model = "gpt-5-codex"
model_provider = "aiguard"

[model_providers.aiguard]
name = "AIGuard"
base_url = "https://api.example.com/v1"
wire_api = "responses"
requires_openai_auth = true
`)
}

// TestEditCodexTOMLIgnoresAnArrayOfTables makes sure [[profiles]]'s own "name"
// key is never mistaken for [model_providers.aiguard]'s.
func TestEditCodexTOMLIgnoresAnArrayOfTables(t *testing.T) {
	original := `[[model_providers.aiguard]]
name = "an array, not aiguard's table"
`

	updated, err := editCodexTOML([]byte(original), relayEdit("https://api.example.com/v1", "gpt-5-codex"))
	if err != nil {
		// go-toml refuses to redefine an array of tables as a table, which is
		// the correct outcome: aiguard reports it rather than mangling the file.
		if !strings.Contains(err.Error(), "config.toml") {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if strings.Contains(string(updated), `name = "an array, not aiguard's table"`) &&
		!strings.Contains(string(updated), `name = "AIGuard"`) {
		t.Errorf("the array-of-tables entry was edited in place instead of left alone:\n%s", updated)
	}
}

func TestEditCodexTOMLKeepsCRLFLineEndings(t *testing.T) {
	original := "# windows\r\napproval_policy = \"on-request\"\r\n"

	updated, err := editCodexTOML([]byte(original), relayEdit("https://api.example.com/v1", "gpt-5-codex"))
	if err != nil {
		t.Fatalf("editCodexTOML: %v", err)
	}
	if strings.Contains(strings.ReplaceAll(string(updated), "\r\n", ""), "\n") {
		t.Errorf("a CRLF config.toml gained a bare LF line:\n%q", updated)
	}
}

func TestEditCodexTOMLCreatesAConfigFromNothing(t *testing.T) {
	updated, err := editCodexTOML(nil, relayEdit("https://api.example.com/v1", "gpt-5-codex"))
	if err != nil {
		t.Fatalf("editCodexTOML: %v", err)
	}

	assertTOML(t, updated, `model_provider = "aiguard"
model = "gpt-5-codex"

[model_providers.aiguard]
name = "AIGuard"
base_url = "https://api.example.com/v1"
wire_api = "responses"
requires_openai_auth = true
`)
}

func TestEditCodexTOMLRejectsAConfigItCannotParse(t *testing.T) {
	if _, err := editCodexTOML([]byte("model = \n[broken\n"), relayEdit("https://api.example.com", "gpt-5-codex")); err == nil {
		t.Error("editCodexTOML should refuse to edit a config.toml go-toml cannot parse")
	}
}

// TestCodexLLMSwitchKeepsConfigFormatting is the round trip through the real
// switcher, so the formatting guarantee is pinned at the level an operator
// sees it rather than only on the editor underneath.
func TestCodexLLMSwitchKeepsConfigFormatting(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	target := codexTestTarget(t)

	configPath := filepath.Join(codexHome, "config.toml")
	original := `# my Codex config
approval_policy = "on-request"

# Docs lookup, do not remove.
[mcp_servers.docs]
command = "npx"
args = ["-y", "@my/docs-server"]   # pinned on purpose
`
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ApplyProvider(target, LLMProvider{
		BaseUrl: "https://api.example.com/v1",
		ApiKey:  "sk-test-123",
		Model:   "gpt-5-codex",
	}); err != nil {
		t.Fatalf("ApplyProvider: %v", err)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# my Codex config\n",
		"# Docs lookup, do not remove.\n",
		`args = ["-y", "@my/docs-server"]   # pinned on purpose`,
	} {
		if !strings.Contains(string(after), want) {
			t.Errorf("switching providers rewrote a line aiguard does not own - %q is gone:\n%s", want, after)
		}
	}

	if err := ClearProvider(target); err != nil {
		t.Fatalf("ClearProvider: %v", err)
	}

	restored, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != original {
		t.Errorf("apply then clear should leave a config with no model settings of its own byte-identical:\n--- got ---\n%s\n--- want ---\n%s", restored, original)
	}
}
