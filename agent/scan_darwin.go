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
	"strconv"
	"strings"
	"syscall"
)

type darwinHome struct {
	owner string
	path  string
}

func scanPlatform() []Installation {
	homes := darwinHomes()
	installations := scanClaudeCode(homes)
	installations = append(installations, scanOpenClaw(homes)...)
	return installations
}

// scanClaudeCode finds supported Claude Code installations in known macOS
// layouts without executing discovered binaries.
func scanClaudeCode(homes []darwinHome) []Installation {
	var installations []Installation
	for _, home := range homes {
		installations = append(installations, scanDarwinNative(home)...)
		installations = append(installations, scanDarwinNpm(home)...)
	}
	for _, prefix := range []string{"/opt/homebrew", "/usr/local"} {
		installations = append(installations, scanDarwinHomebrew(prefix)...)
	}
	installations = append(installations, scanDarwinSystemNpm()...)
	return installations
}

func darwinHomes() []darwinHome {
	seen := map[string]bool{}
	var homes []darwinHome
	add := func(owner, path string) {
		path = filepath.Clean(path)
		if info, err := os.Stat(path); err != nil || !info.IsDir() || seen[path] {
			return
		}
		seen[path] = true
		if owner == "" {
			owner = filepath.Base(path)
		}
		homes = append(homes, darwinHome{owner: owner, path: path})
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

func scanDarwinNative(home darwinHome) []Installation {
	launcher := filepath.Join(home.path, ".local", "bin", "claude")
	info, err := os.Stat(launcher)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return nil
	}

	version := ""
	if target, err := filepath.EvalSymlinks(launcher); err == nil {
		versionsDir := filepath.Join(home.path, ".local", "share", "claude", "versions")
		if relative, err := filepath.Rel(versionsDir, target); err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			version = strings.Split(relative, string(filepath.Separator))[0]
		}
	}
	return []Installation{{Name: "Claude Code", Version: version, Path: launcher, InstallMethod: "native", Owner: home.owner}}
}

func scanDarwinHomebrew(prefix string) []Installation {
	var result []Installation
	for _, cask := range []string{"claude-code", "claude-code@latest"} {
		versions, _ := filepath.Glob(filepath.Join(prefix, "Caskroom", cask, "*"))
		for _, versionDir := range versions {
			executable := filepath.Join(versionDir, "claude")
			info, err := os.Stat(executable)
			if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
				continue
			}
			result = append(result, Installation{
				Name: "Claude Code", Version: filepath.Base(versionDir), Path: executable, InstallMethod: "homebrew", Owner: darwinFileOwner(executable),
			})
		}
	}
	return result
}

func scanDarwinNpm(home darwinHome) []Installation {
	return scanDarwinNpmPatterns([]string{
		filepath.Join(home.path, ".nvm", "versions", "node", "*", "lib", "node_modules", "@anthropic-ai", "claude-code", "package.json"),
		filepath.Join(home.path, ".local", "share", "fnm", "node-versions", "*", "installation", "lib", "node_modules", "@anthropic-ai", "claude-code", "package.json"),
		filepath.Join(home.path, ".volta", "tools", "image", "packages", "@anthropic-ai", "claude-code", "lib", "node_modules", "@anthropic-ai", "claude-code", "package.json"),
		filepath.Join(home.path, ".asdf", "installs", "nodejs", "*", "lib", "node_modules", "@anthropic-ai", "claude-code", "package.json"),
	}, home.owner, "@anthropic-ai/claude-code", "Claude Code")
}

func scanDarwinSystemNpm() []Installation {
	return scanDarwinNpmPatterns([]string{
		"/opt/homebrew/lib/node_modules/@anthropic-ai/claude-code/package.json",
		"/usr/local/lib/node_modules/@anthropic-ai/claude-code/package.json",
	}, "", "@anthropic-ai/claude-code", "Claude Code")
}

func scanDarwinNpmPatterns(patterns []string, owner, packageName, agentName string) []Installation {
	var result []Installation
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, packageJSON := range matches {
			version, ok := readPackageVersion(packageJSON, packageName)
			if !ok {
				continue
			}
			packageOwner := owner
			if packageOwner == "" {
				packageOwner = darwinFileOwner(packageJSON)
			}
			result = append(result, Installation{
				Name: agentName, Version: version, Path: filepath.Dir(packageJSON), InstallMethod: "npm", Owner: packageOwner,
			})
		}
	}
	return result
}

func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

func darwinFileOwner(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "root"
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "root"
	}
	id := strconv.FormatUint(uint64(stat.Uid), 10)
	account, err := user.LookupId(id)
	if err != nil {
		return id
	}
	return account.Username
}
