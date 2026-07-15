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

var darwinSystemNpmTemplates = []string{
	"/opt/homebrew/lib/node_modules/{package}/package.json",
	"/usr/local/lib/node_modules/{package}/package.json",
}

func scanPlatform() []Installation {
	homes := darwinHomes()
	var installations []Installation
	for _, home := range homes {
		values := pathValues{platform: "unix", home: home.path}
		installations = append(installations, scanNpmInstallations(unixUserNpmTemplates, values, home.owner)...)
		installations = append(installations, scanNativeInstallations(home, false)...)
		installations = append(installations, scanGitInstallations(home, false)...)
	}
	installations = append(installations, scanNpmInstallations(darwinSystemNpmTemplates, pathValues{platform: "unix"}, "")...)
	for _, prefix := range []string{"/opt/homebrew", "/usr/local"} {
		installations = append(installations, scanHomebrewInstallations(prefix)...)
	}
	return installations
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
