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
	"unsafe"

	"golang.org/x/sys/windows"
)

// maxImagePathLen is the widest path QueryFullProcessImageName may return; the
// classic MAX_PATH is too small for long-path executables.
const maxImagePathLen = 32768

// enumerateProcesses returns every running process's owner and executable path.
// It reads each process's image path and token, never executing a discovered
// binary; a process we cannot open - typically one owned by another account
// without the rights to inspect it - is skipped.
func enumerateProcesses() []processInfo {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return nil
	}

	var processes []processInfo
	for {
		if path, owner := inspectProcess(entry.ProcessID); path != "" {
			processes = append(processes, processInfo{Path: path, Owner: owner})
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			break
		}
	}
	return processes
}

// inspectProcess opens a process once and reads both its image path and the
// account it runs as. An empty path means the process could not be inspected.
func inspectProcess(pid uint32) (string, string) {
	if pid == 0 {
		return "", ""
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", ""
	}
	defer windows.CloseHandle(handle)

	buf := make([]uint16, maxImagePathLen)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil {
		return "", ""
	}
	return windows.UTF16ToString(buf[:size]), processTokenOwner(handle)
}

// processTokenOwner resolves the account name a process runs as, or "" when it
// cannot be determined. The bare account name (not domain-qualified) matches how
// the filesystem scanners report owners.
func processTokenOwner(process windows.Handle) string {
	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return ""
	}
	defer token.Close()

	user, err := token.GetTokenUser()
	if err != nil {
		return ""
	}
	account, _, _, err := user.User.Sid.LookupAccount("")
	if err != nil {
		return ""
	}
	return account
}
