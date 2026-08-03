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
	"bytes"
	"debug/buildinfo"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// maxVersionFileSize bounds how much of a version file is read, since it is only
// ever expected to hold a short version string on its first line.
const maxVersionFileSize = 4 * 1024

// pseudoVersion matches a Go module pseudo-version's embedded commit timestamp
// and short hash. A pseudo-version identifies a build between tags, not a
// release, so it is never worth showing as "the version".
var pseudoVersion = regexp.MustCompile(`\d{14}-[0-9a-f]{12}`)

// describeSuffix matches the "-<commits>-g<hash>" tail that git describe appends
// to a tag for a build made after it. Trimming it (and a trailing "-dirty")
// collapses a describe version back to its release number.
var describeSuffix = regexp.MustCompile(`-\d+-g[0-9a-f]+$`)

// executableBuildVersion recovers an agent's own release version from a Go
// binary's embedded build metadata. It reads the file, never executing it,
// which matters for agents whose --version flag has side effects (older
// OpenAgent started its server instead of printing a version).
//
// It prefers the version the agent stamps into a package variable through
// -ldflags -X, because that is the version the agent reports about itself (what
// its --version prints). Only when that is absent does it fall back to the Go
// module version, and then only when the module version is a clean release tag
// rather than a "(devel)" or pseudo-version placeholder.
func executableBuildVersion(path, module, versionVar string) string {
	if path == "" || module == "" {
		return ""
	}
	info, err := buildinfo.ReadFile(path)
	if err != nil || info == nil || info.Main.Path != module {
		return ""
	}
	if versionVar != "" {
		for _, setting := range info.Settings {
			if setting.Key != "-ldflags" {
				continue
			}
			if version := ldflagsVersion(setting.Value, versionVar); version != "" {
				return version
			}
		}
	}
	return cleanReleaseVersion(info.Main.Version)
}

// ldflagsVersion extracts the value a -X flag assigns to versionVar inside a
// -ldflags build setting, e.g. "-s -w -X pkg/cli.Version=2.85.0" -> "2.85.0".
func ldflagsVersion(ldflags, versionVar string) string {
	prefix := versionVar + "="
	for _, field := range strings.Fields(ldflags) {
		// A -X flag reaches the linker as either "-X pkg.Var=val" (two fields)
		// or "-X=pkg.Var=val" (one); the assignment token is the same either way.
		field = strings.TrimPrefix(field, "-X=")
		if value, ok := strings.CutPrefix(field, prefix); ok {
			return sanitizeVersion(value)
		}
	}
	return ""
}

// cleanReleaseVersion accepts a Go module version only when it is a real release
// tag: not the "(devel)" placeholder, not a pseudo-version, and not a build
// carrying local modifications (+dirty) or incompatible-module (+incompatible)
// metadata.
func cleanReleaseVersion(version string) string {
	version = sanitizeVersion(version)
	if version == "" || strings.Contains(version, "+") || pseudoVersion.MatchString(version) {
		return ""
	}
	return version
}

// sanitizeVersion drops the placeholder strings that mean "no real version" and
// strips a leading "v" so a version reads as 2.85.0, matching how the other
// agents' versions are shown.
func sanitizeVersion(version string) string {
	switch version = strings.TrimSpace(version); version {
	case "", "dev", "unknown", "(devel)":
		return ""
	}
	if len(version) > 1 && version[0] == 'v' && version[1] >= '0' && version[1] <= '9' {
		version = version[1:]
	}
	// Collapse a git-describe version (tag-<commits>-g<hash>[-dirty]) to its tag,
	// so a build made after a tag, or from a dirty tree, still shows the release
	// number rather than the raw describe string.
	version = strings.TrimSuffix(version, "-dirty")
	version = describeSuffix.ReplaceAllString(version, "")
	return version
}

// executableVersionFile reads a version an agent wrote to a plain-text file next
// to its binary. It is the fallback for release binaries whose build metadata is
// stripped (-trimpath) or unreadable (UPX): the running binary knows its own
// version and records it there, so this recovers it without executing anything.
func executableVersionFile(binaryPath, fileName string) string {
	if binaryPath == "" || fileName == "" {
		return ""
	}
	resolved := binaryPath
	if target, err := filepath.EvalSymlinks(binaryPath); err == nil {
		resolved = target
	}
	path := filepath.Join(filepath.Dir(resolved), fileName)

	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxVersionFileSize {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if newline := bytes.IndexByte(data, '\n'); newline >= 0 {
		data = data[:newline]
	}
	return sanitizeVersion(string(data))
}

// fillMissingVersions labels installations discovered for one fingerprint whose
// version the layout scan could not determine. It reads the binary's embedded
// build metadata first, then a version file the agent may have written next to
// itself, and leaves an already-known version untouched.
func fillMissingVersions(installations []Installation, mark int, fingerprint *compiledFingerprint) {
	if fingerprint.BuildInfoModule == "" && fingerprint.VersionFile == "" {
		return
	}
	for i := mark; i < len(installations); i++ {
		if installations[i].Version != "" {
			continue
		}
		version := executableBuildVersion(
			installations[i].Path, fingerprint.BuildInfoModule, fingerprint.BuildInfoVersionVar)
		if version == "" {
			version = executableVersionFile(installations[i].Path, fingerprint.VersionFile)
		}
		installations[i].Version = version
	}
}
