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

import "testing"

// TestIdentifyExecutable pins the paths the hand-written matcher recognized
// before detection became fingerprint-driven, so a change to the derivation
// rules cannot silently stop recognizing an installed agent.
func TestIdentifyExecutable(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		// Claude Code, per-user native installer.
		{"/home/alice/.local/share/claude/versions/1.2.3/claude", "claude-code"},
		{"/home/alice/.local/bin/claude", "claude-code"},
		{`C:\Users\alice\.local\bin\claude.exe`, "claude-code"},
		// Claude Code, package managers.
		{"/opt/homebrew/Caskroom/claude-code/1.2.3/claude", "claude-code"},
		{"/usr/local/Caskroom/claude-code@latest/1.2.3/claude", "claude-code"},
		{"/usr/lib/node_modules/@anthropic-ai/claude-code/cli.js", "claude-code"},
		{"/usr/lib/node_modules/@anthropic-ai/claude-code-1.2.3/cli.js", "claude-code"},
		{`C:\Users\alice\AppData\Local\Microsoft\WinGet\Packages\Anthropic.ClaudeCode_abc123\claude.exe`, "claude-code"},
		// Claude Code, system bin directories.
		{"/usr/bin/claude", "claude-code"},
		{"/usr/local/bin/claude", "claude-code"},
		{"/home/linuxbrew/.linuxbrew/bin/claude", "claude-code"},
		{"/opt/homebrew/bin/claude", "claude-code"},
		// Claude Desktop.
		{`C:\Users\alice\AppData\Local\AnthropicClaude\claude.exe`, "claude-desktop"},
		{`C:\Users\alice\AppData\Local\AnthropicClaude\app-1.2.3\claude.exe`, "claude-desktop"},
		{`C:\Program Files\WindowsApps\Claude_1.2.3_x64__pzs8sxrjxfjjc\app\claude.exe`, "claude-desktop"},
		// OpenClaw, whose only installation layout is its npm package.
		{"/home/alice/.npm-global/lib/node_modules/openclaw/dist/cli.js", "openclaw"},
		{`C:\Users\alice\AppData\Roaming\npm\node_modules\openclaw\dist\cli.js`, "openclaw"},
		// Codex CLI, whose Windows installer manages a tree of its own and
		// re-executes the launcher out of a content-addressed subdirectory.
		{`C:\Users\alice\AppData\Local\OpenAI\Codex\bin\codex.exe`, "codex-cli"},
		{`C:\Users\alice\AppData\Local\OpenAI\Codex\bin\3135b80b111fd431\codex.exe`, "codex-cli"},
		{"/home/alice/.npm-global/lib/node_modules/@openai/codex/bin/codex.js", "codex-cli"},
		// Cursor, installed by its setup installer machine-wide or per-user.
		{`C:\Program Files\cursor\Cursor.exe`, "cursor"},
		{`C:\Program Files (x86)\cursor\Cursor.exe`, "cursor"},
		{`C:\Users\alice\AppData\Local\Programs\cursor\Cursor.exe`, "cursor"},
		{"/opt/homebrew/Caskroom/cursor/1.2.3/Cursor.app/Contents/MacOS/Cursor", "cursor"},
		// Cursor Agent, whose CLI installs separately from the editor.
		{`C:\Program Files\cursor-agent\cursor-agent.exe`, "cursor-agent"},
		{"/home/alice/.local/share/cursor-agent/versions/1.2.3/cursor-agent", "cursor-agent"},
		{"/home/alice/.local/bin/cursor-agent", "cursor-agent"},
		// Windsurf.
		{`C:\Users\alice\AppData\Local\Programs\Windsurf\Windsurf.exe`, "windsurf"},
		// OpenAgent, whose one-step installer keeps a single binary in a flat
		// ~/.local/share tree with a launcher symlinked onto PATH, and drops the
		// same binary under %LOCALAPPDATA% on Windows.
		{"/home/alice/.local/share/openagent/openagent", "openagent"},
		{"/home/alice/.local/bin/openagent", "openagent"},
		{`C:\Users\alice\AppData\Local\openagent\openagent.exe`, "openagent"},
		// The single binary keeps its name wherever it runs, so a custom install
		// dir or a build from a source checkout is recognized too.
		{"/opt/openagent/openagent", "openagent"},
		{"/home/alice/src/openagent/openagent", "openagent"},
		{`D:\work\openagent\openagent.exe`, "openagent"},
		// Unrelated executables must stay unrecognized.
		{"/usr/bin/node", ""},
		{"/usr/bin/bash", ""},
		{"/home/alice/claude", ""},
		{"/home/alice/.local/bin/claude-helper", ""},
		{"/home/alice/openclaw/dist/cli.js", ""},
		{"", ""},
	}

	for _, test := range tests {
		if got := IdentifyExecutable(test.path); got != test.want {
			t.Errorf("IdentifyExecutable(%q) = %q, want %q", test.path, got, test.want)
		}
	}
}

// TestIdentifyExecutableMachineWinget covers the machine-wide winget root, which
// the previous hand-written matcher missed even though the scanner searched it.
func TestIdentifyExecutableMachineWinget(t *testing.T) {
	path := `C:\Program Files\WinGet\Packages\Anthropic.ClaudeCode_abc123\claude.exe`
	if got := IdentifyExecutable(path); got != "claude-code" {
		t.Errorf("IdentifyExecutable(%q) = %q, want %q", path, got, "claude-code")
	}
}

// TestIdentifyExecutableDesktopSubdirectory covers desktop installer layouts
// that nest the executable under something other than the "app-<version>"
// directory the previous hand-written matcher assumed.
func TestIdentifyExecutableDesktopSubdirectory(t *testing.T) {
	path := `C:\Users\alice\AppData\Local\AnthropicClaude\current\claude.exe`
	if got := IdentifyExecutable(path); got != "claude-desktop" {
		t.Errorf("IdentifyExecutable(%q) = %q, want %q", path, got, "claude-desktop")
	}
}

func TestIdentifyCmdline(t *testing.T) {
	tests := []struct {
		cmdline string
		want    string
	}{
		{"node /usr/lib/node_modules/openclaw/dist/cli.js", "openclaw"},
		{"node /usr/lib/node_modules/OpenClaw/dist/cli.js", "openclaw"},
		{"node /srv/app/server.js", ""},
		{"", ""},
	}

	for _, test := range tests {
		if got := IdentifyCmdline(test.cmdline); got != test.want {
			t.Errorf("IdentifyCmdline(%q) = %q, want %q", test.cmdline, got, test.want)
		}
	}
}

// TestFingerprintsAreDistinct guards the registry itself: two agents that claim
// the same ID, or a fingerprint carrying neither path nor cmdline evidence,
// would make detection results ambiguous or dead.
func TestFingerprintsAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, fingerprint := range compiledFingerprints {
		if fingerprint.ID == "" || fingerprint.DisplayName == "" {
			t.Errorf("fingerprint %+v is missing an ID or DisplayName", fingerprint.Fingerprint)
		}
		if seen[fingerprint.ID] {
			t.Errorf("duplicate fingerprint ID %q", fingerprint.ID)
		}
		seen[fingerprint.ID] = true
		if len(fingerprint.execRules) == 0 && len(fingerprint.cmdMarkers) == 0 {
			t.Errorf("fingerprint %q can never match: no exec rules and no cmdline markers", fingerprint.ID)
		}
	}
}

// TestPathRuleZeroValueNeverMatches ensures an under-specified rule cannot match
// every path, which would misattribute unrelated processes to an agent.
func TestPathRuleZeroValueNeverMatches(t *testing.T) {
	if (PathRule{}).matches("/usr/bin/node") {
		t.Error("zero PathRule matched a path; it must never match")
	}
}
