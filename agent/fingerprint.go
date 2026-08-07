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
	pathpkg "path"
	"strings"

	"github.com/casdoor/casdoor-aiguard/localserver"
)

// Fingerprint is the complete per-agent data set. Everything that differs
// between agents lives here; the discovery and identification logic in this
// package is shared and reads only these fields. Adding support for a new
// agent should mean adding one file to fingerprints/, not new scan code.
//
// It carries no behaviour, only data, which is why the registry ships as one
// JSON file per agent rather than as Go literals: this struct is the schema of
// those files, and its field comments document them. See fingerprint_load.go
// for how they are read and what the loader rejects.
//
// Every field is optional: a scanner skips an agent whose relevant field is
// empty, so a cmdline-only agent needs nothing but ID, DisplayName and
// CmdMarkers.
type Fingerprint struct {
	// ID is the stable identifier reported for a recognized process, and
	// DisplayName is the human-readable name reported for an installation. ID
	// must also be the base name of the file the fingerprint is read from.
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`

	// ExecName is the launcher's base name, without the Windows ".exe".
	ExecName string `json:"execName,omitempty"`
	// StateDir is the directory under ~/.local/share holding the versioned
	// payloads of the per-user native installer.
	StateDir string `json:"stateDir,omitempty"`
	// NpmPackage is the exact "name" field of the published npm package.
	NpmPackage string `json:"npmPackage,omitempty"`
	// ExtraUnixNpmDirs are agent-specific directories, relative to the user's
	// home and slash-separated, that hold a "node_modules" alongside a bundled
	// Node runtime. ExtraWindowsNpmDirs is the same, relative to %LOCALAPPDATA%.
	// Both may contain glob wildcards. Agents installed only through a shared
	// Node version manager need neither.
	ExtraUnixNpmDirs    []string `json:"extraUnixNpmDirs,omitempty"`
	ExtraWindowsNpmDirs []string `json:"extraWindowsNpmDirs,omitempty"`
	// WingetPackage is the winget package identifier, without its hash suffix.
	WingetPackage string `json:"wingetPackage,omitempty"`
	// MSIXFamily is the Windows package family name ("<Name>_<PublisherId>").
	MSIXFamily string `json:"msixFamily,omitempty"`
	// DesktopInstallerDir is the directory under %LOCALAPPDATA% used by the
	// standalone desktop installer.
	DesktopInstallerDir string `json:"desktopInstallerDir,omitempty"`
	// WindowsProgramDirs are the installation directories of a Windows
	// setup-style installer, each slash-separated and relative to a Program
	// Files root - and to %LOCALAPPDATA%\Programs, where the very same
	// installer lands when it is run for a single user instead of the machine.
	WindowsProgramDirs []string `json:"windowsProgramDirs,omitempty"`
	// WindowsUserDirs are installation directories relative to %LOCALAPPDATA%,
	// slash-separated, for per-user installers that lay out their own tree
	// rather than reusing the Programs directory.
	WindowsUserDirs []string `json:"windowsUserDirs,omitempty"`
	// HomebrewCasks lists the cask names that install this agent.
	HomebrewCasks []string `json:"homebrewCasks,omitempty"`
	// SystemPackage is the package name used by apt, rpm and apk alike.
	SystemPackage string `json:"systemPackage,omitempty"`
	// BuildInfoModule is the Go main-module path of an agent built in Go. When
	// set, a scan reads the binary's embedded build metadata (without executing
	// it) to recover a version an install layout with no versioned directory or
	// manifest cannot supply.
	BuildInfoModule string `json:"buildInfoModule,omitempty"`
	// BuildInfoVersionVar is the -ldflags -X variable the agent stamps its own
	// version into, preferred over the module version because it is the version
	// the agent reports about itself.
	BuildInfoVersionVar string `json:"buildInfoVersionVar,omitempty"`
	// VersionFile is a plain-text file, next to the binary, whose first line is
	// the version. It is the fallback for release binaries stripped of build
	// metadata (-trimpath, UPX), which the agent writes there itself at startup.
	VersionFile string `json:"versionFile,omitempty"`

	// CmdMarkers are substrings that identify the agent in a process command
	// line. Runtimes like Node.js report a generic comm ("node"), so the agent
	// is only visible in the argv or script path.
	CmdMarkers []string `json:"cmdMarkers,omitempty"`
	// ExtraExecRules covers executable layouts that the fields above do not
	// imply. Most agents need none.
	ExtraExecRules []PathRule `json:"extraExecRules,omitempty"`

	// LocalServer describes the HTTP server the agent runs on the loopback
	// interface, so a scan can find it by the port it answers on even when its
	// binary sits outside every layout above. Nil for agents that run no
	// server; see the localserver package for what the fields mean.
	LocalServer *localserver.Server `json:"localServer,omitempty"`

	// Notes is documentation, never read by any scan: it is where the reasoning
	// that used to sit in a comment beside a Go literal goes, now that the
	// registry is JSON and cannot carry comments. Anything explaining *why* an
	// entry looks the way it does belongs here, next to the data it explains.
	Notes []string `json:"notes,omitempty"`
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
// matches, and the loader rejects one rather than shipping a rule that is
// silently dead.
type PathRule struct {
	Exact    string   `json:"exact,omitempty"`
	Suffix   string   `json:"suffix,omitempty"`
	Contains []string `json:"contains,omitempty"`
}

// isEmpty reports a rule that constrains nothing. Such a rule never matches, so
// in a data file it is always a mistake.
func (r PathRule) isEmpty() bool {
	return r.Exact == "" && r.Suffix == "" && len(r.Contains) == 0
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
	// filepath.Clean only recognizes separators for the host OS. Explicitly
	// fold Windows separators first so Windows fixtures and remote process
	// paths match correctly even when aiguard is built or tested on Unix.
	normalized := strings.ToLower(pathpkg.Clean(strings.ReplaceAll(path, `\`, "/")))
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
