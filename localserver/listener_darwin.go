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

package localserver

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// macOS exposes no readable socket table: /proc does not exist, and the libproc
// call behind it needs cgo. So the listener is resolved through the two system
// tools that already wrap it - lsof for the socket's owner, ps for what that
// process is - both invoked at an absolute path, so neither can be shadowed by
// something earlier on PATH.
const (
	lsofPath              = "/usr/sbin/lsof"
	psPath                = "/bin/ps"
	listenerCommandBudget = 3 * time.Second
)

// Listeners returns the processes listening on port. A process whose executable
// cannot be read - one owned by another user, most often - is left out, since a
// listener with no path is of no use to a caller.
func Listeners(port int) []Process {
	// -t prints bare PIDs, one per line, of the listening sockets alone.
	out, ok := listenerCommand(lsofPath, "-nP", "-t", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN")
	if !ok {
		return nil
	}

	var result []Process
	seen := map[int]bool{}
	for _, line := range strings.Fields(out) {
		pid, err := strconv.Atoi(line)
		if err != nil || pid <= 0 || seen[pid] {
			continue
		}
		seen[pid] = true

		// On macOS "comm" is the executable's full path, not just its name.
		path, ok := listenerCommand(psPath, "-o", "comm=", "-p", line)
		path = strings.TrimSpace(path)
		if !ok || !filepath.IsAbs(path) {
			continue
		}
		owner, _ := listenerCommand(psPath, "-o", "user=", "-p", line)
		result = append(result, Process{Pid: pid, Path: path, Owner: strings.TrimSpace(owner)})
	}
	return result
}

func listenerCommand(path string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), listenerCommandBudget)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, args...).Output()
	return string(out), err == nil
}
