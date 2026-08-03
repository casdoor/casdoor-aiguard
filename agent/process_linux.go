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
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// enumerateProcesses returns every running process's owner and executable path,
// read from /proc. This feeds the Agents inventory and is separate from the
// interception path, which resolves a process only when its traffic is caught:
// without this, a running source build would be missing from the Agents page
// until it happened to make an intercepted network call. A process whose exe we
// cannot read - another account's without the rights, or already gone - is
// skipped.
func enumerateProcesses() []processInfo {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	var processes []processInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue // not a <pid> directory
		}

		procDir := filepath.Join("/proc", entry.Name())
		exe, err := os.Readlink(filepath.Join(procDir, "exe"))
		if err != nil || exe == "" {
			continue
		}
		// A process whose binary was replaced or removed reads back with a
		// " (deleted)" marker; the real path is what precedes it.
		exe = strings.TrimSuffix(exe, " (deleted)")
		processes = append(processes, processInfo{Path: exe, Owner: fileOwner(procDir)})
	}
	return processes
}
