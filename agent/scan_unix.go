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

var unixUserNpmTemplates = []string{
	"{home}/.npm-global/lib/node_modules/{package}/package.json",
	"{home}/.nvm/versions/node/*/lib/node_modules/{package}/package.json",
	"{home}/.fnm/node-versions/*/installation/lib/node_modules/{package}/package.json",
	"{home}/.local/share/fnm/node-versions/*/installation/lib/node_modules/{package}/package.json",
	"{home}/.volta/tools/image/packages/{package}/lib/node_modules/{package}/package.json",
	"{home}/.asdf/installs/nodejs/*/lib/node_modules/{package}/package.json",
}

func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

func pathOwner(path string) string {
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
