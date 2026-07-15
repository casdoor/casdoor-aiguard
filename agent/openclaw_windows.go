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
	"path/filepath"
)

func scanOpenClaw(homes []windowsHome) []Installation {
	var installations []Installation
	for _, home := range homes {
		installations = append(installations, scanWindowsOpenClawNpm(home)...)
		installations = append(installations, scanWindowsOpenClawGit(home)...)
	}
	return installations
}

func scanWindowsOpenClawNpm(home windowsHome) []Installation {
	roaming, local := windowsDataDirs(home)
	return scanWindowsNpmPatterns([]string{
		filepath.Join(roaming, "npm", "node_modules", "openclaw", "package.json"),
		filepath.Join(roaming, "nvm", "*", "node_modules", "openclaw", "package.json"),
		filepath.Join(roaming, "fnm", "node-versions", "*", "installation", "node_modules", "openclaw", "package.json"),
		filepath.Join(local, "Volta", "tools", "image", "packages", "openclaw", "node_modules", "openclaw", "package.json"),
		filepath.Join(local, "OpenClaw", "deps", "portable-node", "node_modules", "openclaw", "package.json"),
	}, home.owner, "openclaw", "OpenClaw")
}

func scanWindowsOpenClawGit(home windowsHome) []Installation {
	wrapper := filepath.Join(home.path, ".local", "bin", "openclaw.cmd")
	if info, err := os.Stat(wrapper); err != nil || !info.Mode().IsRegular() {
		return nil
	}

	repo := filepath.Join(home.path, "openclaw")
	version, ok := readPackageVersion(filepath.Join(repo, "package.json"), "openclaw")
	if !ok {
		return nil
	}
	return []Installation{{Name: "OpenClaw", Version: version, Path: repo, InstallMethod: "git", Owner: home.owner}}
}
