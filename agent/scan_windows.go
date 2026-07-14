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
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/windows/registry"
)

type windowsHome struct {
	owner string
	path  string
}

// Scan finds supported Claude Code installations in known Windows layouts.
// It neither executes discovered binaries nor walks arbitrary filesystem roots.
func Scan() []Installation {
	homes := windowsHomes()
	var installations []Installation
	for _, home := range homes {
		installations = append(installations, scanWindowsNative(home)...)
		installations = append(installations, scanWindowsWinget(home)...)
		installations = append(installations, scanWindowsNpm(home)...)
	}
	installations = append(installations, scanMachineWinget()...)

	seen := map[string]bool{}
	result := installations[:0]
	for _, installation := range installations {
		key := strings.ToLower(windowsCanonicalPath(installation.Path))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, installation)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Owner != result[j].Owner {
			return result[i].Owner < result[j].Owner
		}
		return result[i].Path < result[j].Path
	})
	return result
}

func windowsHomes() []windowsHome {
	seen := map[string]bool{}
	var homes []windowsHome
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
		homes = append(homes, windowsHome{owner: owner, path: path})
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

func scanWindowsNative(home windowsHome) []Installation {
	launcher := filepath.Join(home.path, ".local", "bin", "claude.exe")
	launcherInfo, err := os.Stat(launcher)
	if err != nil || !launcherInfo.Mode().IsRegular() {
		return nil
	}

	version := ""
	versionsDir := filepath.Join(home.path, ".local", "share", "claude", "versions")
	if target, err := filepath.EvalSymlinks(launcher); err == nil {
		if relative, err := filepath.Rel(versionsDir, target); err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			version = strings.Split(relative, string(filepath.Separator))[0]
		}
	}
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
	return []Installation{{Name: "Claude Code", Version: version, Path: launcher, InstallMethod: "native", Owner: home.owner}}
}

func scanWindowsWinget(home windowsHome) []Installation {
	localAppData := filepath.Join(home.path, "AppData", "Local")
	if current, err := os.UserHomeDir(); err == nil && strings.EqualFold(filepath.Clean(current), filepath.Clean(home.path)) {
		if configured := os.Getenv("LOCALAPPDATA"); configured != "" {
			localAppData = configured
		}
	}
	root := filepath.Join(localAppData, "Microsoft", "WinGet", "Packages")
	return scanWingetPackages(root, home.owner)
}

func scanMachineWinget() []Installation {
	var result []Installation
	seen := map[string]bool{}
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		base := os.Getenv(env)
		if base == "" {
			continue
		}
		root := filepath.Join(base, "WinGet", "Packages")
		key := strings.ToLower(filepath.Clean(root))
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, scanWingetPackages(root, "SYSTEM")...)
	}
	return result
}

func scanWingetPackages(root, owner string) []Installation {
	packages, _ := filepath.Glob(filepath.Join(root, "Anthropic.ClaudeCode_*"))
	var result []Installation
	for _, packageDir := range packages {
		executable := filepath.Join(packageDir, "claude.exe")
		if info, err := os.Stat(executable); err == nil && info.Mode().IsRegular() {
			result = append(result, Installation{Name: "Claude Code", Path: executable, InstallMethod: "winget", Owner: owner})
		}
	}
	return result
}

func scanWindowsNpm(home windowsHome) []Installation {
	roaming := filepath.Join(home.path, "AppData", "Roaming")
	local := filepath.Join(home.path, "AppData", "Local")
	if current, err := os.UserHomeDir(); err == nil && strings.EqualFold(filepath.Clean(current), filepath.Clean(home.path)) {
		if configured := os.Getenv("APPDATA"); configured != "" {
			roaming = configured
		}
		if configured := os.Getenv("LOCALAPPDATA"); configured != "" {
			local = configured
		}
	}
	patterns := []string{
		filepath.Join(roaming, "npm", "node_modules", "@anthropic-ai", "claude-code", "package.json"),
		filepath.Join(roaming, "nvm", "*", "node_modules", "@anthropic-ai", "claude-code", "package.json"),
		filepath.Join(roaming, "fnm", "node-versions", "*", "installation", "node_modules", "@anthropic-ai", "claude-code", "package.json"),
		filepath.Join(local, "Volta", "tools", "image", "packages", "@anthropic-ai", "claude-code", "lib", "node_modules", "@anthropic-ai", "claude-code", "package.json"),
	}
	return scanWindowsNpmPatterns(patterns, home.owner)
}

func scanWindowsNpmPatterns(patterns []string, owner string) []Installation {
	var result []Installation
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, packageJSON := range matches {
			info, err := os.Stat(packageJSON)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1024*1024 {
				continue
			}
			data, err := os.ReadFile(packageJSON)
			if err != nil {
				continue
			}
			var pkg struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}
			if json.Unmarshal(data, &pkg) != nil || pkg.Name != "@anthropic-ai/claude-code" || pkg.Version == "" {
				continue
			}
			result = append(result, Installation{Name: "Claude Code", Version: pkg.Version, Path: filepath.Dir(packageJSON), InstallMethod: "npm", Owner: owner})
		}
	}
	return result
}

func windowsCanonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}
