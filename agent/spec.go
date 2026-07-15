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

type agentSpec struct {
	Name                   string
	ID                     string
	Binary                 string
	NpmPackage             string
	NpmPaths               []npmPathSpec
	Native                 *nativeSpec
	Git                    *gitSpec
	HomebrewCasks          []string
	LinuxPackage           string
	WingetID               string
	WindowsDesktop         *windowsDesktopSpec
	ExecutablePaths        []string
	ExecutableFingerprints []pathFingerprint
}

type npmPathSpec struct {
	Platform string
	Template string
}

type nativeSpec struct {
	UnixLauncher    string
	WindowsLauncher string
	VersionsDir     string
}

type gitSpec struct {
	RepoDir         string
	UnixWrappers    []string
	WindowsWrappers []string
}

type windowsDesktopSpec struct {
	InstallerPath  string
	PackageFamily  string
	MSIXExecutable string
}

type pathFingerprint struct {
	Contains []string
	Suffix   string
}

var agentSpecs = []agentSpec{
	{
		Name:         "Claude Code",
		ID:           "claude-code",
		Binary:       "claude",
		NpmPackage:   "@anthropic-ai/claude-code",
		LinuxPackage: "claude-code",
		WingetID:     "Anthropic.ClaudeCode",
		Native: &nativeSpec{
			UnixLauncher:    ".local/bin/claude",
			WindowsLauncher: ".local/bin/claude.exe",
			VersionsDir:     ".local/share/claude/versions",
		},
		HomebrewCasks: []string{"claude-code", "claude-code@latest"},
		ExecutablePaths: []string{
			"/usr/bin/claude",
			"/usr/local/bin/claude",
			"/home/linuxbrew/.linuxbrew/bin/claude",
			"/opt/homebrew/bin/claude",
		},
		ExecutableFingerprints: []pathFingerprint{
			{Contains: []string{"/.local/share/claude/versions/"}},
			{Suffix: "/.local/bin/claude"},
			{Suffix: "/.local/bin/claude.exe"},
			{Contains: []string{"/caskroom/claude-code/"}},
			{Contains: []string{"/caskroom/claude-code@latest/"}},
			{Contains: []string{"/microsoft/winget/packages/anthropic.claudecode_"}},
			{Contains: []string{"/node_modules/@anthropic-ai/claude-code-"}},
		},
	},
	{
		Name:       "OpenClaw",
		ID:         "openclaw",
		Binary:     "openclaw",
		NpmPackage: "openclaw",
		NpmPaths: []npmPathSpec{
			{Platform: "unix", Template: "{home}/.openclaw/tools/node-v*/lib/node_modules/{package}/package.json"},
			{Platform: "windows", Template: "{local}/OpenClaw/deps/portable-node/node_modules/{package}/package.json"},
		},
		Git: &gitSpec{
			RepoDir:         "openclaw",
			UnixWrappers:    []string{".local/bin/openclaw", ".openclaw/bin/openclaw"},
			WindowsWrappers: []string{".local/bin/openclaw.cmd"},
		},
	},
	{
		Name: "Claude Desktop",
		ID:   "claude-desktop",
		WindowsDesktop: &windowsDesktopSpec{
			InstallerPath:  "AnthropicClaude/claude.exe",
			PackageFamily:  "Claude_pzs8sxrjxfjjc",
			MSIXExecutable: "app/claude.exe",
		},
		ExecutableFingerprints: []pathFingerprint{
			{Contains: []string{"/appdata/local/anthropicclaude/"}, Suffix: "/claude.exe"},
			{Contains: []string{"/program files/windowsapps/claude_", "__pzs8sxrjxfjjc/app/claude.exe"}},
		},
	},
}
