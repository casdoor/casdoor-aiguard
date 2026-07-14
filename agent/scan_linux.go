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
	"encoding/json"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type homeDir struct {
	owner string
	path  string
}

// Scan finds supported Claude Code installations without executing any
// discovered binary or traversing arbitrary filesystem roots.
func Scan() []Installation {
	installations := make([]Installation, 0)
	homes := readHomes("/etc/passwd")
	for _, home := range homes {
		installations = append(installations, scanNative(home)...)
		installations = append(installations, scanUserNpm(home)...)
	}
	installations = append(installations, scanSystemNpm()...)
	installations = append(installations, scanSystemPackages()...)
	installations = append(installations, scanHomebrew()...)

	seen := map[string]bool{}
	result := installations[:0]
	for _, installation := range installations {
		key := canonicalPath(installation.Path)
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

func scanNative(home homeDir) []Installation {
	launcher := filepath.Join(home.path, ".local", "bin", "claude")
	info, err := os.Stat(launcher)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return nil
	}

	version := ""
	if target, err := filepath.EvalSymlinks(launcher); err == nil {
		versionsDir := filepath.Join(home.path, ".local", "share", "claude", "versions") + string(filepath.Separator)
		if strings.HasPrefix(target, versionsDir) {
			version = filepath.Base(target)
		}
	}
	return []Installation{{Name: "Claude Code", Version: version, Path: launcher, InstallMethod: "native", Owner: home.owner}}
}

func scanUserNpm(home homeDir) []Installation {
	patterns := []string{
		filepath.Join(home.path, ".nvm", "versions", "node", "*", "lib", "node_modules", "@anthropic-ai", "claude-code", "package.json"),
		filepath.Join(home.path, ".local", "share", "fnm", "node-versions", "*", "installation", "lib", "node_modules", "@anthropic-ai", "claude-code", "package.json"),
		filepath.Join(home.path, ".volta", "tools", "image", "packages", "@anthropic-ai", "claude-code", "lib", "node_modules", "@anthropic-ai", "claude-code", "package.json"),
		filepath.Join(home.path, ".asdf", "installs", "nodejs", "*", "lib", "node_modules", "@anthropic-ai", "claude-code", "package.json"),
	}
	return scanNpmPatterns(patterns, home.owner)
}

func scanSystemNpm() []Installation {
	return scanNpmPatterns([]string{
		"/usr/local/lib/node_modules/@anthropic-ai/claude-code/package.json",
		"/usr/lib/node_modules/@anthropic-ai/claude-code/package.json",
	}, "")
}

func scanNpmPatterns(patterns []string, owner string) []Installation {
	var result []Installation
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, packageJSON := range matches {
			info, err := os.Stat(packageJSON)
			if err != nil || info.Size() > 1024*1024 {
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
			packageOwner := owner
			if packageOwner == "" {
				packageOwner = fileOwner(packageJSON)
			}
			result = append(result, Installation{
				Name: "Claude Code", Version: pkg.Version, Path: filepath.Dir(packageJSON), InstallMethod: "npm", Owner: packageOwner,
			})
		}
	}
	return result
}

func scanSystemPackages() []Installation {
	var result []Installation
	if version, ok := commandOutput("dpkg-query", "-W", "-f=${Version}", "claude-code"); ok {
		path, _ := commandOutput("dpkg", "-L", "claude-code")
		result = append(result, packageInstallation("apt", version, path))
	}
	if version, ok := commandOutput("rpm", "-q", "--qf", "%{VERSION}-%{RELEASE}", "claude-code"); ok {
		path, _ := commandOutput("rpm", "-ql", "claude-code")
		result = append(result, packageInstallation("rpm", version, path))
	}
	if installed, ok := commandOutput("apk", "info", "-e", "claude-code"); ok && strings.TrimSpace(installed) != "" {
		version, _ := commandOutput("apk", "info", "-v", "claude-code")
		path, _ := commandOutput("apk", "info", "-L", "claude-code")
		version = strings.TrimPrefix(strings.TrimSpace(version), "claude-code-")
		result = append(result, packageInstallation("apk", version, path))
	}
	return result
}

func packageInstallation(method, version, files string) Installation {
	path := findClaudePath(files)
	if path == "" {
		path = "/usr/bin/claude"
	}
	return Installation{Name: "Claude Code", Version: strings.TrimSpace(version), Path: path, InstallMethod: method, Owner: "root"}
}

func scanHomebrew() []Installation {
	brewPaths := []string{"/home/linuxbrew/.linuxbrew/bin/brew", "/opt/homebrew/bin/brew", "/usr/local/bin/brew"}
	var result []Installation
	for _, brew := range brewPaths {
		if info, err := os.Stat(brew); err != nil || !info.Mode().IsRegular() {
			continue
		}
		owner := fileOwner(brew)
		prefix := filepath.Dir(filepath.Dir(brew))
		result = append(result, scanHomebrewCaskroom(prefix, owner)...)
		for _, cask := range []string{"claude-code", "claude-code@latest"} {
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
			path := findClaudePath(files)
			if path == "" {
				path = filepath.Join(filepath.Dir(brew), "claude")
			}
			result = append(result, Installation{Name: "Claude Code", Version: version, Path: path, InstallMethod: "homebrew", Owner: owner})
		}
	}
	return result
}

func scanHomebrewCaskroom(prefix, owner string) []Installation {
	launcher := filepath.Join(prefix, "bin", "claude")
	target, err := filepath.EvalSymlinks(launcher)
	if err != nil {
		return nil
	}
	for _, cask := range []string{"claude-code", "claude-code@latest"} {
		root := filepath.Join(prefix, "Caskroom", cask)
		relative, err := filepath.Rel(root, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		version := strings.Split(relative, string(filepath.Separator))[0]
		if version != "" && version != "." {
			return []Installation{{Name: "Claude Code", Version: version, Path: launcher, InstallMethod: "homebrew", Owner: owner}}
		}
	}
	return nil
}

func findClaudePath(files string) string {
	for _, line := range strings.Split(files, "\n") {
		line = strings.TrimSpace(line)
		if filepath.Base(line) == "claude" {
			return line
		}
	}
	return ""
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

func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

func fileOwner(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "root"
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "root"
	}
	account, err := user.LookupId(strconv.FormatUint(uint64(stat.Uid), 10))
	if err != nil {
		return strconv.FormatUint(uint64(stat.Uid), 10)
	}
	return account.Username
}
