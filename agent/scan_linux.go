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

//go:build linux

package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Scan finds installations of every known agent without executing any
// discovered binary or traversing arbitrary filesystem roots.
func Scan() []Installation {
	homes := readHomes("/etc/passwd")

	var installations []Installation
	for _, fingerprint := range compiledFingerprints {
		mark := len(installations)
		for _, home := range homes {
			installations = append(installations, scanNative(fingerprint, home)...)
			installations = append(installations, scanNpmPatterns(fingerprint, userNpmPatterns(fingerprint, home.path), home.owner, fileOwner)...)
		}
		installations = append(installations, scanSystemNpm(fingerprint)...)
		installations = append(installations, scanSystemPackages(fingerprint)...)
		installations = append(installations, scanHomebrew(fingerprint)...)
		stampAgentId(installations, mark, fingerprint.ID)
	}
	return dedupeInstallations(installations)
}

func readHomes(passwdPath string) []homeDir {
	data, err := os.ReadFile(passwdPath)
	if err != nil {
		return nil
	}

	seen := map[string]bool{}
	var homes []homeDir
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 6 || !filepath.IsAbs(fields[5]) || seen[fields[5]] {
			continue
		}
		info, err := os.Stat(fields[5])
		if err != nil || !info.IsDir() {
			continue
		}
		seen[fields[5]] = true
		homes = append(homes, homeDir{owner: fields[0], path: fields[5]})
	}
	return homes
}

func scanSystemNpm(fingerprint *compiledFingerprint) []Installation {
	pkg := fingerprint.npmPackagePath()
	return scanNpmPatterns(fingerprint, []string{
		filepath.Join("/usr/local/lib/node_modules", pkg, "package.json"),
		filepath.Join("/usr/lib/node_modules", pkg, "package.json"),
	}, "", fileOwner)
}

// scanSystemPackages queries each distro package manager for the agent, which
// reports both its version and the files it owns.
func scanSystemPackages(fingerprint *compiledFingerprint) []Installation {
	pkg := fingerprint.SystemPackage
	if pkg == "" {
		return nil
	}

	var result []Installation
	if version, ok := commandOutput("dpkg-query", "-W", "-f=${Version}", pkg); ok {
		files, _ := commandOutput("dpkg", "-L", pkg)
		result = append(result, packageInstallation(fingerprint, "apt", version, files))
	}
	if version, ok := commandOutput("rpm", "-q", "--qf", "%{VERSION}-%{RELEASE}", pkg); ok {
		files, _ := commandOutput("rpm", "-ql", pkg)
		result = append(result, packageInstallation(fingerprint, "rpm", version, files))
	}
	if installed, ok := commandOutput("apk", "info", "-e", pkg); ok && strings.TrimSpace(installed) != "" {
		version, _ := commandOutput("apk", "info", "-v", pkg)
		files, _ := commandOutput("apk", "info", "-L", pkg)
		version = strings.TrimPrefix(strings.TrimSpace(version), pkg+"-")
		result = append(result, packageInstallation(fingerprint, "apk", version, files))
	}
	return result
}

func packageInstallation(fingerprint *compiledFingerprint, method, version, files string) Installation {
	path := findExecutablePath(files, fingerprint.ExecName)
	if path == "" {
		path = filepath.Join("/usr/bin", fingerprint.ExecName)
	}
	return Installation{
		Name: fingerprint.DisplayName, Version: strings.TrimSpace(version), Path: path,
		InstallMethod: method, Owner: "root",
	}
}

func scanHomebrew(fingerprint *compiledFingerprint) []Installation {
	if len(fingerprint.HomebrewCasks) == 0 {
		return nil
	}

	var result []Installation
	for _, brew := range []string{"/home/linuxbrew/.linuxbrew/bin/brew", "/opt/homebrew/bin/brew", "/usr/local/bin/brew"} {
		if info, err := os.Stat(brew); err != nil || !info.Mode().IsRegular() {
			continue
		}
		owner := fileOwner(brew)
		prefix := filepath.Dir(filepath.Dir(brew))
		result = append(result, scanHomebrewCaskroom(fingerprint, prefix, owner)...)
		for _, cask := range fingerprint.HomebrewCasks {
			out, ok := commandOutputPath(brew, "list", "--cask", "--versions", cask)
			if !ok || strings.TrimSpace(out) == "" {
				continue
			}
			fields := strings.Fields(out)
			version := ""
			if len(fields) > 1 {
				version = fields[len(fields)-1]
			}
			files, _ := commandOutputPath(brew, "list", "--cask", cask)
			path := findExecutablePath(files, fingerprint.ExecName)
			if path == "" {
				path = filepath.Join(filepath.Dir(brew), fingerprint.ExecName)
			}
			result = append(result, Installation{
				Name: fingerprint.DisplayName, Version: version, Path: path,
				InstallMethod: "homebrew", Owner: owner,
			})
		}
	}
	return result
}

// scanHomebrewCaskroom reads the version straight from the Caskroom layout, so
// a cask still resolves when the brew command itself is unavailable.
func scanHomebrewCaskroom(fingerprint *compiledFingerprint, prefix, owner string) []Installation {
	launcher := filepath.Join(prefix, "bin", fingerprint.ExecName)
	target, err := filepath.EvalSymlinks(launcher)
	if err != nil {
		return nil
	}
	for _, cask := range fingerprint.HomebrewCasks {
		version := versionUnderDir(filepath.Join(prefix, "Caskroom", cask), target)
		if version != "" {
			return []Installation{{
				Name: fingerprint.DisplayName, Version: version, Path: launcher,
				InstallMethod: "homebrew", Owner: owner,
			}}
		}
	}
	return nil
}

func commandOutput(name string, args ...string) (string, bool) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	return commandOutputPath(path, args...)
}

func commandOutputPath(path string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, args...).Output()
	return string(out), err == nil
}
