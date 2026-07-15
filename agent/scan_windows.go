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

type windowsHome struct {
	owner string
	path  string
}

func scanPlatform() []Installation {
	homes := windowsHomes()
	installations := scanClaudeCode(homes)
	installations = append(installations, scanOpenClaw(homes)...)
	return installations
}

// scanClaudeCode finds supported Claude Code installations in known Windows
// layouts without executing discovered binaries.
func scanClaudeCode(homes []windowsHome) []Installation {
	var installations []Installation
	for _, home := range homes {
		installations = append(installations, scanWindowsNative(home)...)
		installations = append(installations, scanWindowsWinget(home)...)
		installations = append(installations, scanWindowsNpm(home)...)
	}
	installations = append(installations, scanWindowsDesktop(homes)...)
	installations = append(installations, scanMachineWinget()...)
	return installations
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
	roaming, local := windowsDataDirs(home)
	patterns := []string{
		filepath.Join(roaming, "npm", "node_modules", "@anthropic-ai", "claude-code", "package.json"),
		filepath.Join(roaming, "nvm", "*", "node_modules", "@anthropic-ai", "claude-code", "package.json"),
		filepath.Join(roaming, "fnm", "node-versions", "*", "installation", "node_modules", "@anthropic-ai", "claude-code", "package.json"),
		filepath.Join(local, "Volta", "tools", "image", "packages", "@anthropic-ai", "claude-code", "lib", "node_modules", "@anthropic-ai", "claude-code", "package.json"),
	}
	return scanWindowsNpmPatterns(patterns, home.owner, "@anthropic-ai/claude-code", "Claude Code")
}

func windowsDataDirs(home windowsHome) (string, string) {
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
	return roaming, local
}

func scanWindowsNpmPatterns(patterns []string, owner, packageName, agentName string) []Installation {
	var result []Installation
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, packageJSON := range matches {
			version, ok := readPackageVersion(packageJSON, packageName)
			if !ok {
				continue
			}
			result = append(result, Installation{Name: agentName, Version: version, Path: filepath.Dir(packageJSON), InstallMethod: "npm", Owner: owner})
		}
	}
	return result
}

func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return strings.ToLower(resolved)
	}
	return strings.ToLower(filepath.Clean(path))
}
