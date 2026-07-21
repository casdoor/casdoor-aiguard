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

//go:build windows

package agent

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// homeDir is one user's profile directory and the account that owns it.
type homeDir struct {
	owner string
	path  string
}

// Scan finds installations of every known agent in known Windows layouts. It
// neither executes discovered binaries nor walks arbitrary filesystem roots.
func Scan() []Installation {
	homes := windowsHomes()

	var installations []Installation
	for _, fingerprint := range compiledFingerprints {
		for _, home := range homes {
			installations = append(installations, scanWindowsNative(fingerprint, home)...)
			installations = append(installations, scanWindowsWinget(fingerprint, home)...)
			installations = append(installations, scanWindowsNpm(fingerprint, home)...)
		}
		installations = append(installations, scanWindowsDesktop(fingerprint, homes)...)
		installations = append(installations, scanMachineWinget(fingerprint)...)
	}
	return dedupeInstallations(installations)
}

func windowsHomes() []homeDir {
	seen := map[string]bool{}
	var homes []homeDir
	add := func(owner, path string) {
		path = filepath.Clean(path)
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			return
		}
		key := strings.ToLower(path)
		if seen[key] {
			return
		}
		seen[key] = true
		if owner == "" {
			owner = filepath.Base(path)
		}
		homes = append(homes, homeDir{owner: owner, path: path})
	}

	const profileList = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList`
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, profileList, registry.ENUMERATE_SUB_KEYS)
	if err == nil {
		defer key.Close()
		if sids, err := key.ReadSubKeyNames(-1); err == nil {
			for _, sid := range sids {
				profile, err := registry.OpenKey(key, sid, registry.QUERY_VALUE)
				if err != nil {
					continue
				}
				path, _, readErr := profile.GetStringValue("ProfileImagePath")
				profile.Close()
				if readErr != nil {
					continue
				}
				if expanded, err := registry.ExpandString(path); err == nil {
					path = expanded
				}
				owner := ""
				if account, err := user.LookupId(sid); err == nil {
					owner = account.Username
				}
				add(owner, path)
			}
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		owner := ""
		if account, err := user.Current(); err == nil {
			owner = account.Username
		}
		add(owner, home)
	}
	return homes
}

// appDataDir resolves one of a profile's AppData subdirectories. For the
// current user it prefers the environment variable, which respects a relocated
// profile; for other users only the conventional layout is available.
func appDataDir(home homeDir, kind, variable string) string {
	if current, err := os.UserHomeDir(); err == nil && strings.EqualFold(filepath.Clean(current), filepath.Clean(home.path)) {
		if configured := os.Getenv(variable); configured != "" {
			return configured
		}
	}
	return filepath.Join(home.path, "AppData", kind)
}

func localAppData(home homeDir) string   { return appDataDir(home, "Local", "LOCALAPPDATA") }
func roamingAppData(home homeDir) string { return appDataDir(home, "Roaming", "APPDATA") }

// scanWindowsNative reports the per-user native installer layout: a launcher at
// %USERPROFILE%\.local\bin\<exec>.exe backed by a versioned payload directory.
func scanWindowsNative(fingerprint *compiledFingerprint, home homeDir) []Installation {
	if fingerprint.ExecName == "" || fingerprint.StateDir == "" {
		return nil
	}

	launcher := filepath.Join(home.path, ".local", "bin", fingerprint.ExecName+".exe")
	launcherInfo, err := os.Stat(launcher)
	if err != nil || !launcherInfo.Mode().IsRegular() {
		return nil
	}

	version := ""
	versionsDir := filepath.Join(home.path, ".local", "share", fingerprint.StateDir, "versions")
	if target, err := filepath.EvalSymlinks(launcher); err == nil {
		version = versionUnderDir(versionsDir, target)
	}
	// Windows installs may hard-link rather than symlink the launcher, in which
	// case the version is only recoverable by identity against the payloads.
	if version == "" {
		if entries, err := os.ReadDir(versionsDir); err == nil {
			for _, entry := range entries {
				candidate, err := entry.Info()
				if err == nil && candidate.Mode().IsRegular() && os.SameFile(launcherInfo, candidate) {
					version = entry.Name()
					break
				}
			}
		}
	}
	return []Installation{{
		Name: fingerprint.DisplayName, Version: version, Path: launcher,
		InstallMethod: "native", Owner: home.owner,
	}}
}

func scanWindowsWinget(fingerprint *compiledFingerprint, home homeDir) []Installation {
	root := filepath.Join(localAppData(home), "Microsoft", "WinGet", "Packages")
	return scanWingetPackages(fingerprint, root, home.owner)
}

func scanMachineWinget(fingerprint *compiledFingerprint) []Installation {
	var result []Installation
	seen := map[string]bool{}
	for _, variable := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		base := os.Getenv(variable)
		if base == "" {
			continue
		}
		root := filepath.Join(base, "WinGet", "Packages")
		key := strings.ToLower(filepath.Clean(root))
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, scanWingetPackages(fingerprint, root, "SYSTEM")...)
	}
	return result
}

func scanWingetPackages(fingerprint *compiledFingerprint, root, owner string) []Installation {
	if fingerprint.WingetPackage == "" || fingerprint.ExecName == "" {
		return nil
	}

	// winget appends an install-source hash to the package identifier.
	packages, _ := filepath.Glob(filepath.Join(root, fingerprint.WingetPackage+"_*"))
	var result []Installation
	for _, packageDir := range packages {
		executable := filepath.Join(packageDir, fingerprint.ExecName+".exe")
		if info, err := os.Stat(executable); err == nil && info.Mode().IsRegular() {
			result = append(result, Installation{
				Name: fingerprint.DisplayName, Path: executable,
				InstallMethod: "winget", Owner: owner,
			})
		}
	}
	return result
}

func scanWindowsNpm(fingerprint *compiledFingerprint, home homeDir) []Installation {
	roaming := roamingAppData(home)
	local := localAppData(home)
	pkg := fingerprint.npmPackagePath()
	patterns := []string{
		filepath.Join(roaming, "npm", "node_modules", pkg, "package.json"),
		filepath.Join(roaming, "nvm", "*", "node_modules", pkg, "package.json"),
		filepath.Join(roaming, "fnm", "node-versions", "*", "installation", "node_modules", pkg, "package.json"),
		// A Volta package image mirrors the npm prefix layout, which on Windows
		// has no "lib" level; match both so a relocated image still resolves.
		filepath.Join(local, "Volta", "tools", "image", "packages", pkg, "node_modules", pkg, "package.json"),
		filepath.Join(local, "Volta", "tools", "image", "packages", pkg, "lib", "node_modules", pkg, "package.json"),
		// pnpm groups its global packages under a store layout version.
		filepath.Join(local, "pnpm", "global", "*", "node_modules", pkg, "package.json"),
	}
	for _, dir := range fingerprint.ExtraWindowsNpmDirs {
		patterns = append(patterns, filepath.Join(local, filepath.FromSlash(dir), "node_modules", pkg, "package.json"))
	}
	return scanNpmPatterns(fingerprint, patterns, home.owner, nil)
}
