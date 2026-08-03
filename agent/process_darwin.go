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
	"context"
	"os/exec"
	"strings"
	"time"
)

const processListTimeout = 3 * time.Second

// enumerateProcesses returns every running process's owner and executable path.
// It shells out to ps rather than executing any discovered agent: on macOS
// `ps -o comm` prints the full executable path, `-o user` the owning account,
// and -ww stops either being truncated to the terminal width. A line we cannot
// parse is skipped.
func enumerateProcesses() []processInfo {
	ctx, cancel := context.WithTimeout(context.Background(), processListTimeout)
	defer cancel()

	// user first (a single whitespace-free token), then the path, which may
	// itself contain spaces - so split on the first gap only.
	output, err := exec.CommandContext(ctx, "ps", "-axww", "-o", "user=", "-o", "comm=").Output()
	if err != nil {
		return nil
	}

	var processes []processInfo
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		gap := strings.IndexAny(line, " \t")
		if gap <= 0 {
			continue
		}
		path := strings.TrimSpace(line[gap:])
		if path == "" {
			continue
		}
		processes = append(processes, processInfo{Owner: line[:gap], Path: path})
	}
	return processes
}
