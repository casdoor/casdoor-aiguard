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

var windowsUserNpmTemplates = []string{
	"{roaming}/npm/node_modules/{package}/package.json",
	"{roaming}/nvm/*/node_modules/{package}/package.json",
	"{roaming}/fnm/node-versions/*/installation/node_modules/{package}/package.json",
	"{local}/Volta/tools/image/packages/{package}/node_modules/{package}/package.json",
}

func scanPlatform() []Installation {
	homes := windowsHomes()
	var installations []Installation
	for _, home := range homes {
		roaming, local := windowsDataDirs(home)
		values := pathValues{platform: "windows", home: home.path, roaming: roaming, local: local}
		installations = append(installations, scanNpmInstallations(windowsUserNpmTemplates, values, home.owner)...)
		installations = append(installations, scanNativeInstallations(home, true)...)
		installations = append(installations, scanGitInstallations(home, true)...)
		installations = append(installations, scanWingetPackages(filepath.Join(local, "Microsoft", "WinGet", "Packages"), home.owner)...)
	}
	installations = append(installations, scanWindowsDesktop(homes)...)
	installations = append(installations, scanMachineWinget()...)
	return installations
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

func windowsDataDirs(home homeDir) (string, string) {
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

func scanWingetPackages(root, owner string) []Installation {
	var result []Installation
	for _, spec := range agentSpecs {
		if spec.WingetID == "" {
			continue
		}
		packages, _ := filepath.Glob(filepath.Join(root, spec.WingetID+"_*"))
		for _, packageDir := range packages {
			executable := filepath.Join(packageDir, spec.Binary+".exe")
			if info, err := os.Stat(executable); err == nil && info.Mode().IsRegular() {
				result = append(result, Installation{Name: spec.Name, Path: executable, InstallMethod: "winget", Owner: owner})
			}
		}
	}
	return result
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

func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return strings.ToLower(resolved)
	}
	return strings.ToLower(filepath.Clean(path))
}

func pathOwner(string) string {
	return "SYSTEM"
}
