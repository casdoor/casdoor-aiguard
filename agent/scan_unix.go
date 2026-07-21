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
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

// homeDir is one user's home directory and the account that owns it.
type homeDir struct {
	owner string
	path  string
}

// scanNative reports the per-user native installer layout: a launcher at
// ~/.local/bin/<exec> pointing into ~/.local/share/<state>/versions/<version>.
func scanNative(fingerprint *compiledFingerprint, home homeDir) []Installation {
	if fingerprint.ExecName == "" || fingerprint.StateDir == "" {
		return nil
	}

	launcher := filepath.Join(home.path, ".local", "bin", fingerprint.ExecName)
	info, err := os.Stat(launcher)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return nil
	}

	version := ""
	if target, err := filepath.EvalSymlinks(launcher); err == nil {
		version = versionUnderDir(filepath.Join(home.path, ".local", "share", fingerprint.StateDir, "versions"), target)
	}
	return []Installation{{
		Name: fingerprint.DisplayName, Version: version, Path: launcher,
		InstallMethod: "native", Owner: home.owner,
	}}
}

// userNpmPatterns are the per-user Node version manager layouts that can hold a
// globally installed agent package.
func userNpmPatterns(fingerprint *compiledFingerprint, home string) []string {
	pkg := fingerprint.npmPackagePath()
	return []string{
		filepath.Join(home, ".nvm", "versions", "node", "*", "lib", "node_modules", pkg, "package.json"),
		filepath.Join(home, ".local", "share", "fnm", "node-versions", "*", "installation", "lib", "node_modules", pkg, "package.json"),
		filepath.Join(home, ".volta", "tools", "image", "packages", pkg, "lib", "node_modules", pkg, "package.json"),
		filepath.Join(home, ".asdf", "installs", "nodejs", "*", "lib", "node_modules", pkg, "package.json"),
	}
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
	id := strconv.FormatUint(uint64(stat.Uid), 10)
	account, err := user.LookupId(id)
	if err != nil {
		return id
	}
	return account.Username
}
