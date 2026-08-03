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

//go:build darwin

package agent

import (
	"os"
	"os/user"
	"path/filepath"
)

// Scan finds installations of every known agent in known macOS layouts. It
// neither executes discovered binaries nor walks arbitrary filesystem roots.
func Scan() []Installation {
	homes := darwinHomes()

	var installations []Installation
	for _, fingerprint := range compiledFingerprints {
		mark := len(installations)
		for _, home := range homes {
			installations = append(installations, scanNative(fingerprint, home)...)
			installations = append(installations, scanNpmPatterns(fingerprint, userNpmPatterns(fingerprint, home.path), home.owner, fileOwner)...)
		}
		for _, prefix := range []string{"/opt/homebrew", "/usr/local"} {
			installations = append(installations, scanDarwinHomebrew(fingerprint, prefix)...)
			installations = append(installations, scanDarwinSystemNpm(fingerprint, prefix)...)
		}
		stampAgentId(installations, mark, fingerprint.ID)
		fillMissingVersions(installations, mark, fingerprint)
	}
	installations = append(installations, scanCodexStandalone()...)
	installations = append(installations, scanCodexDarwinApps(homes)...)
	return expandSharedCodexInstallations(dedupeInstallations(installations), homes)
}

func darwinHomes() []homeDir {
	seen := map[string]bool{}
	var homes []homeDir
	add := func(owner, path string) {
		path = filepath.Clean(path)
		if info, err := os.Stat(path); err != nil || !info.IsDir() || seen[path] {
			return
		}
		seen[path] = true
		if owner == "" {
			owner = filepath.Base(path)
		}
		homes = append(homes, homeDir{owner: owner, path: path})
	}

	if entries, err := os.ReadDir("/Users"); err == nil {
		for _, entry := range entries {
			if entry.Name() != "Shared" {
				add(entry.Name(), filepath.Join("/Users", entry.Name()))
			}
		}
	}
	add("root", "/var/root")
	if home, err := os.UserHomeDir(); err == nil {
		owner := ""
		if account, err := user.Current(); err == nil {
			owner = account.Username
		}
		add(owner, home)
	}
	return homes
}

// scanDarwinHomebrew reads the Caskroom layout directly; on macOS every cask
// version keeps its own directory, so no brew invocation is needed.
func scanDarwinHomebrew(fingerprint *compiledFingerprint, prefix string) []Installation {
	if fingerprint.ExecName == "" {
		return nil
	}

	var result []Installation
	for _, cask := range fingerprint.HomebrewCasks {
		versions, _ := filepath.Glob(filepath.Join(prefix, "Caskroom", cask, "*"))
		for _, versionDir := range versions {
			executable := filepath.Join(versionDir, fingerprint.ExecName)
			info, err := os.Stat(executable)
			if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
				continue
			}
			result = append(result, Installation{
				Name: fingerprint.DisplayName, Version: filepath.Base(versionDir), Path: executable,
				InstallMethod: "homebrew", Owner: fileOwner(executable),
			})
		}
	}
	return result
}

func scanDarwinSystemNpm(fingerprint *compiledFingerprint, prefix string) []Installation {
	pattern := filepath.Join(prefix, "lib", "node_modules", fingerprint.npmPackagePath(), "package.json")
	return scanNpmPatterns(fingerprint, []string{pattern}, "", fileOwner)
}
