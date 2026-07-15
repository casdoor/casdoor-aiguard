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

//go:build linux || darwin

package agent

import (
	"os"
	"path/filepath"
)

func openClawNpmPatterns(home string) []string {
	return []string{
		filepath.Join(home, ".npm-global", "lib", "node_modules", "openclaw", "package.json"),
		filepath.Join(home, ".nvm", "versions", "node", "*", "lib", "node_modules", "openclaw", "package.json"),
		filepath.Join(home, ".fnm", "node-versions", "*", "installation", "lib", "node_modules", "openclaw", "package.json"),
		filepath.Join(home, ".local", "share", "fnm", "node-versions", "*", "installation", "lib", "node_modules", "openclaw", "package.json"),
		filepath.Join(home, ".volta", "tools", "image", "packages", "openclaw", "lib", "node_modules", "openclaw", "package.json"),
		filepath.Join(home, ".openclaw", "tools", "node", "lib", "node_modules", "openclaw", "package.json"),
	}
}

func scanOpenClawGit(home, owner string) []Installation {
	installed := false
	for _, wrapper := range []string{
		filepath.Join(home, ".local", "bin", "openclaw"),
		filepath.Join(home, ".openclaw", "bin", "openclaw"),
	} {
		if info, err := os.Stat(wrapper); err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			installed = true
			break
		}
	}
	if !installed {
		return nil
	}

	repo := filepath.Join(home, "openclaw")
	version, ok := readPackageVersion(filepath.Join(repo, "package.json"), "openclaw")
	if !ok {
		return nil
	}
	return []Installation{{Name: "OpenClaw", Version: version, Path: repo, InstallMethod: "git", Owner: owner}}
}
