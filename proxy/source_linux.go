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

package proxy

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/casdoor/casdoor-aiguard/agent"
)

// LookupSourceProcess best-effort resolves the PID, process name and
// recognized agent of the local process that owns localAddr (the client side
// of an intercepted connection), by cross-referencing /proc/net/tcp[6] socket
// inodes against /proc/<pid>/fd symlinks. Full SPIFFE-based identity binding is
// a later stage.
func LookupSourceProcess(localAddr net.Addr) (pid int, name string, agent string) {
	tcpAddr, ok := localAddr.(*net.TCPAddr)
	if !ok {
		return 0, "", ""
	}

	inode := findSocketInode(tcpAddr)
	if inode == "" {
		return 0, "", ""
	}

	pid = findPidForInode(inode)
	if pid == 0 {
		return 0, "", ""
	}

	name = readProcessName(pid)
	agent = identifyAgent(pid)
	return pid, name, agent
}

// maxAgentParentWalk bounds how far up the process tree identifyAgent looks.
// The outbound LLM call can come from a forked worker whose own argv is just
// "node", while the marker lives in an ancestor (the openclaw gateway).
const maxAgentParentWalk = 8

// identifyAgent walks the process and its ancestors, matching each executable
// path and command line against the agent fingerprints, and returns the
// recognized agent ID ("" if none).
func identifyAgent(pid int) string {
	for i := 0; i < maxAgentParentWalk && pid > 1; i++ {
		if name := agent.IdentifyExecutable(readProcessExecutable(pid)); name != "" {
			return name
		}
		if name := agent.IdentifyCmdline(readProcessCmdline(pid)); name != "" {
			return name
		}
		ppid := readParentPid(pid)
		if ppid == pid || ppid == 0 {
			break
		}
		pid = ppid
	}
	return ""
}

func readProcessExecutable(pid int) string {
	path, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return ""
	}
	return path
}

func findSocketInode(addr *net.TCPAddr) string {
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		if inode := scanProcNetTcp(path, addr); inode != "" {
			return inode
		}
	}
	return ""
}

func scanProcNetTcp(path string, addr *net.TCPAddr) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	wantPort := fmt.Sprintf("%04X", addr.Port)

	scanner := bufio.NewScanner(f)
	scanner.Scan() // header
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			continue
		}
		localParts := strings.Split(fields[1], ":")
		if len(localParts) != 2 {
			continue
		}
		if !strings.EqualFold(localParts[1], wantPort) {
			continue
		}
		return fields[9] // inode column
	}
	return ""
}

func findPidForInode(inode string) int {
	target := fmt.Sprintf("socket:[%s]", inode)

	procDirs, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, entry := range procDirs {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if link == target {
				return pid
			}
		}
	}
	return 0
}

func readProcessName(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// readProcessCmdline returns the full argv of pid with the NUL separators
// replaced by spaces (e.g. "node /usr/lib/node_modules/openclaw/dist/cli.js").
func readProcessCmdline(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(string(data), "\x00", " "))
}

// readParentPid returns the parent PID of pid, parsed from /proc/<pid>/stat.
// The comm field there is wrapped in parentheses and may contain spaces, so we
// key off the closing paren rather than splitting the whole line.
func readParentPid(pid int) int {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	s := string(data)
	rparen := strings.LastIndexByte(s, ')')
	if rparen < 0 {
		return 0
	}
	// After ") " come: state (field 3) then ppid (field 4).
	fields := strings.Fields(s[rparen+1:])
	if len(fields) < 2 {
		return 0
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0
	}
	return ppid
}
