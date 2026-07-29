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
	"path/filepath"
	"strings"
)

// Fingerprint is the complete per-agent data set. Everything that differs
// between agents lives here; the discovery and identification logic in this
// package is shared and reads only these fields. Adding support for a new
// agent should mean adding one entry to fingerprints, not new scan code.
//
// Every field is optional: a scanner skips an agent whose relevant field is
// empty, so a cmdline-only agent needs nothing but Name and CmdMarkers.
type Fingerprint struct {
	// ID is the stable identifier reported for a recognized process, and
	// DisplayName is the human-readable name reported for an installation.
	ID          string
	DisplayName string

	// ExecName is the launcher's base name, without the Windows ".exe".
	ExecName string
	// StateDir is the directory under ~/.local/share holding the versioned
	// payloads of the per-user native installer.
	StateDir string
	// NpmPackage is the exact "name" field of the published npm package.
	NpmPackage string
	// ExtraUnixNpmDirs are agent-specific directories, relative to the user's
	// home and slash-separated, that hold a "node_modules" alongside a bundled
	// Node runtime. ExtraWindowsNpmDirs is the same, relative to %LOCALAPPDATA%.
	// Both may contain glob wildcards. Agents installed only through a shared
	// Node version manager need neither.
	ExtraUnixNpmDirs    []string
	ExtraWindowsNpmDirs []string
	// WingetPackage is the winget package identifier, without its hash suffix.
	WingetPackage string
	// MSIXFamily is the Windows package family name ("<Name>_<PublisherId>").
	MSIXFamily string
	// DesktopInstallerDir is the directory under %LOCALAPPDATA% used by the
	// standalone desktop installer.
	DesktopInstallerDir string
	// WindowsProgramDirs are the installation directories of a Windows
	// setup-style installer, each slash-separated and relative to a Program
	// Files root - and to %LOCALAPPDATA%\Programs, where the very same
	// installer lands when it is run for a single user instead of the machine.
	WindowsProgramDirs []string
	// WindowsUserDirs are installation directories relative to %LOCALAPPDATA%,
	// slash-separated, for per-user installers that lay out their own tree
	// rather than reusing the Programs directory.
	WindowsUserDirs []string
	// HomebrewCasks lists the cask names that install this agent.
	HomebrewCasks []string
	// SystemPackage is the package name used by apt, rpm and apk alike.
	SystemPackage string

	// CmdMarkers are substrings that identify the agent in a process command
	// line. Runtimes like Node.js report a generic comm ("node"), so the agent
	// is only visible in the argv or script path.
	CmdMarkers []string
	// ExtraExecRules covers executable layouts that the fields above do not
	// imply. Most agents need none.
	ExtraExecRules []PathRule
}

// fingerprints is the registry of agents aiguard knows how to recognize.
var fingerprints = []Fingerprint{
	{
		ID:            "claude-code",
		DisplayName:   "Claude Code",
		ExecName:      "claude",
		StateDir:      "claude",
		NpmPackage:    "@anthropic-ai/claude-code",
		WingetPackage: "Anthropic.ClaudeCode",
		HomebrewCasks: []string{"claude-code", "claude-code@latest"},
		SystemPackage: "claude-code",
	},
	{
		ID:                  "claude-desktop",
		DisplayName:         "Claude Desktop",
		ExecName:            "claude",
		MSIXFamily:          "Claude_pzs8sxrjxfjjc",
		DesktopInstallerDir: "AnthropicClaude",
	},
	{
		ID:          "openclaw",
		DisplayName: "OpenClaw",
		ExecName:    "openclaw",
		NpmPackage:  "openclaw",
		// OpenClaw can bring its own Node runtime, which puts the package
		// outside every shared version manager layout.
		ExtraUnixNpmDirs:    []string{".openclaw/tools/node-v*/lib"},
		ExtraWindowsNpmDirs: []string{"OpenClaw/deps/portable-node"},
		CmdMarkers:          []string{"openclaw"},
	},
	{
		ID:          "codex-cli",
		DisplayName: "Codex CLI",
		ExecName:    "codex",
		NpmPackage:  "@openai/codex",
		// The Codex installer keeps the launcher, its helpers and a bundled
		// Node runtime in one bin directory of its own.
		WindowsUserDirs: []string{"OpenAI/Codex/bin"},
	},
	{
		ID:          "hermes-agent",
		DisplayName: "Hermes Agent",
		ExecName:    "hermes",
		CmdMarkers:  []string{"hermes_cli", "/hermes-agent/hermes", `\hermes-agent\hermes`},
		ExtraExecRules: []PathRule{
			{Suffix: "/.local/bin/hermes"},
			{Exact: "/usr/local/bin/hermes"},
			{Contains: []string{"/hermes-agent/venv/bin/python"}},
			{Contains: []string{"/hermes-agent/venv/scripts/python"}},
			{Contains: []string{"/hermes/hermes-agent/venv/scripts/hermes.exe"}},
		},
	},
	{
		ID:          "cursor",
		DisplayName: "Cursor",
		ExecName:    "Cursor",
		// Cursor ships a Windows setup installer that writes the whole editor
		// into one directory, machine-wide or per-user.
		WindowsProgramDirs: []string{"cursor"},
		HomebrewCasks:      []string{"cursor"},
	},
	{
		ID:          "cursor-agent",
		DisplayName: "Cursor Agent",
		ExecName:    "cursor-agent",
		StateDir:    "cursor-agent",
		// The CLI has its own installer on Windows, separate from the editor's.
		WindowsProgramDirs: []string{"cursor-agent"},
		CmdMarkers:         []string{"cursor-agent"},
	},
	{
		ID:                 "windsurf",
		DisplayName:        "Windsurf",
		ExecName:           "Windsurf",
		WindowsProgramDirs: []string{"Windsurf"},
		HomebrewCasks:      []string{"windsurf"},
	},
}

// Directories that hold an agent launcher once a system or Homebrew package
// manager has installed it.
var (
	systemBinDirs   = []string{"/usr/bin", "/usr/local/bin"}
	homebrewBinDirs = []string{"/home/linuxbrew/.linuxbrew/bin", "/opt/homebrew/bin"}
)

// windowsProgramRoots are the roots a Windows setup installer offers to install
// into: the two machine-wide ones, and the per-user one it falls back to when
// it cannot elevate. They appear here as normalized path fragments; the scanner
// resolves the real directories from the environment.
var windowsProgramRoots = []string{
	"/program files",
	"/program files (x86)",
	"/appdata/local/programs",
}

// PathRule matches a normalized executable path: lowercased, forward-slashed
// and cleaned. A rule matches when every constraint it sets holds, so a rule
// can require both a directory marker and a file name. The zero rule never
// matches.
type PathRule struct {
	Exact    string
	Suffix   string
	Contains []string
}

func (r PathRule) matches(path string) bool {
	if r.Exact != "" {
		return path == r.Exact
	}
	if r.Suffix == "" && len(r.Contains) == 0 {
		return false
	}
	if r.Suffix != "" && !strings.HasSuffix(path, r.Suffix) {
		return false
	}
	for _, contains := range r.Contains {
		if !strings.Contains(path, contains) {
			return false
		}
	}
	return true
}

// compiledFingerprint pairs a Fingerprint with the match rules derived from it.
type compiledFingerprint struct {
	Fingerprint
	execRules  []PathRule
	cmdMarkers []string
}

var compiledFingerprints = compileFingerprints(fingerprints)

func compileFingerprints(entries []Fingerprint) []*compiledFingerprint {
	compiled := make([]*compiledFingerprint, 0, len(entries))
	for _, entry := range entries {
		markers := make([]string, 0, len(entry.CmdMarkers))
		for _, marker := range entry.CmdMarkers {
			markers = append(markers, strings.ToLower(marker))
		}
		compiled = append(compiled, &compiledFingerprint{
			Fingerprint: entry,
			execRules:   deriveExecRules(entry),
			cmdMarkers:  markers,
		})
	}
	return compiled
}

// deriveExecRules turns the installation data of a fingerprint into the path
// rules that recognize a running executable. Deriving instead of hand-listing
// keeps the two halves of detection - where we find an agent on disk, and how
// we recognize it once it runs - from drifting apart.
func deriveExecRules(f Fingerprint) []PathRule {
	if f.ExecName == "" {
		return f.ExtraExecRules
	}

	var rules []PathRule
	exec := strings.ToLower(f.ExecName)
	windowsExec := exec + ".exe"

	if f.StateDir != "" {
		rules = append(rules,
			PathRule{Contains: []string{"/.local/share/" + strings.ToLower(f.StateDir) + "/versions/"}},
			PathRule{Suffix: "/.local/bin/" + exec},
			PathRule{Suffix: "/.local/bin/" + windowsExec},
		)
	}
	for _, cask := range f.HomebrewCasks {
		rules = append(rules, PathRule{Contains: []string{"/caskroom/" + strings.ToLower(cask) + "/"}})
	}
	if len(f.HomebrewCasks) > 0 {
		for _, dir := range homebrewBinDirs {
			rules = append(rules, PathRule{Exact: dir + "/" + exec})
		}
	}
	if f.SystemPackage != "" {
		for _, dir := range systemBinDirs {
			rules = append(rules, PathRule{Exact: dir + "/" + exec})
		}
	}
	if f.WingetPackage != "" {
		// Covers both the per-user (%LOCALAPPDATA%\Microsoft\WinGet) and the
		// machine-wide (%ProgramFiles%\WinGet) package roots.
		rules = append(rules, PathRule{Contains: []string{"/winget/packages/" + strings.ToLower(f.WingetPackage) + "_"}})
	}
	if f.NpmPackage != "" {
		npm := "/node_modules/" + strings.ToLower(f.NpmPackage)
		rules = append(rules,
			PathRule{Contains: []string{npm + "/"}},
			PathRule{Contains: []string{npm + "-"}},
		)
	}
	if f.DesktopInstallerDir != "" {
		// The desktop installer keeps each build in its own versioned
		// subdirectory and leaves a launcher at the root, so match the
		// executable at any depth below the install directory.
		root := "/appdata/local/" + strings.ToLower(f.DesktopInstallerDir)
		rules = append(rules, PathRule{Contains: []string{root + "/"}, Suffix: "/" + windowsExec})
	}
	// A setup installer puts the launcher at the root of its install directory
	// and helper executables below it, so match the launcher at any depth: on
	// Windows the process we see is often one of those helpers re-executing the
	// same binary.
	for _, dir := range f.WindowsProgramDirs {
		for _, root := range windowsProgramRoots {
			rules = append(rules, PathRule{
				Contains: []string{root + "/" + strings.ToLower(dir) + "/"},
				Suffix:   "/" + windowsExec,
			})
		}
	}
	for _, dir := range f.WindowsUserDirs {
		rules = append(rules, PathRule{
			Contains: []string{"/appdata/local/" + strings.ToLower(dir) + "/"},
			Suffix:   "/" + windowsExec,
		})
	}
	if name, publisher, ok := splitMSIXFamily(f.MSIXFamily); ok {
		rules = append(rules, PathRule{Contains: []string{
			"/windowsapps/" + name + "_",
			"__" + publisher + "/app/" + windowsExec,
		}})
	}
	return append(rules, f.ExtraExecRules...)
}

// splitMSIXFamily splits a package family name into its lowercased name and
// publisher id, which appear on either side of the version in an installed
// MSIX path.
func splitMSIXFamily(family string) (name string, publisher string, ok bool) {
	separator := strings.LastIndex(family, "_")
	if separator <= 0 || separator == len(family)-1 {
		return "", "", false
	}
	return strings.ToLower(family[:separator]), strings.ToLower(family[separator+1:]), true
}

// IdentifyExecutable returns the agent ID for a known executable layout, or ""
// when the path belongs to no known agent. On Linux, the caller should pass the
// resolved /proc/<pid>/exe path.
func IdentifyExecutable(path string) string {
	if path == "" {
		return ""
	}
	// filepath.ToSlash only replaces the separator of the host OS. Tests,
	// remote process inventories and saved records can still hand a Linux
	// build a Windows path, so normalize backslashes explicitly as well.
	normalized := strings.ToLower(strings.ReplaceAll(filepath.ToSlash(filepath.Clean(path)), `\`, "/"))
	for _, fingerprint := range compiledFingerprints {
		for _, rule := range fingerprint.execRules {
			if rule.matches(normalized) {
				return fingerprint.ID
			}
		}
	}
	return ""
}

// IdentifyCmdline returns the agent ID for a process command line, or "" when
// it matches no known agent.
func IdentifyCmdline(cmdline string) string {
	if cmdline == "" {
		return ""
	}
	normalized := strings.ToLower(cmdline)
	for _, fingerprint := range compiledFingerprints {
		for _, marker := range fingerprint.cmdMarkers {
			if strings.Contains(normalized, marker) {
				return fingerprint.ID
			}
		}
	}
	return ""
}
