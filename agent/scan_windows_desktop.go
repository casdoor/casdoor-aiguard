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
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const claudeDesktopPackageFamily = "Claude_pzs8sxrjxfjjc"

var (
	kernel32                   = windows.NewLazySystemDLL("kernel32.dll")
	getPackagesByPackageFamily = kernel32.NewProc("GetPackagesByPackageFamily")
	getPackagePathByFullName   = kernel32.NewProc("GetPackagePathByFullName")
)

func scanWindowsDesktop(homes []windowsHome) []Installation {
	msix := scanClaudeDesktopMSIX()
	currentHome, _ := os.UserHomeDir()
	result := append([]Installation(nil), msix...)
	for _, home := range homes {
		if len(msix) != 0 && strings.EqualFold(filepath.Clean(home.path), filepath.Clean(currentHome)) {
			continue
		}
		result = append(result, scanClaudeDesktopInstaller(home)...)
	}
	return result
}

func scanClaudeDesktopInstaller(home windowsHome) []Installation {
	localAppData := filepath.Join(home.path, "AppData", "Local")
	if current, err := os.UserHomeDir(); err == nil && strings.EqualFold(filepath.Clean(current), filepath.Clean(home.path)) {
		if configured := os.Getenv("LOCALAPPDATA"); configured != "" {
			localAppData = configured
		}
	}
	executable := filepath.Join(localAppData, "AnthropicClaude", "claude.exe")
	if info, err := os.Stat(executable); err != nil || !info.Mode().IsRegular() {
		return nil
	}
	return []Installation{{Name: "Claude Desktop", Path: executable, InstallMethod: "desktop-installer", Owner: home.owner}}
}

func scanClaudeDesktopMSIX() []Installation {
	fullNames := packageFullNames(claudeDesktopPackageFamily)
	if len(fullNames) == 0 {
		return nil
	}
	owner := ""
	if account, err := user.Current(); err == nil {
		owner = account.Username
	}
	var result []Installation
	for _, fullName := range fullNames {
		if !strings.HasSuffix(strings.ToLower(fullName), "__pzs8sxrjxfjjc") {
			continue
		}
		root := packagePath(fullName)
		if root == "" {
			continue
		}
		executable := filepath.Join(root, "app", "claude.exe")
		fields := strings.Split(fullName, "_")
		version := ""
		if len(fields) > 1 {
			version = fields[1]
		}
		result = append(result, Installation{
			Name: "Claude Desktop", Version: version, Path: executable, InstallMethod: "msix", Owner: owner,
		})
	}
	return result
}

func packageFullNames(family string) []string {
	familyName, err := windows.UTF16PtrFromString(family)
	if err != nil {
		return nil
	}
	var count, bufferLength uint32
	code, _, _ := getPackagesByPackageFamily.Call(
		uintptr(unsafe.Pointer(familyName)), uintptr(unsafe.Pointer(&count)), 0,
		uintptr(unsafe.Pointer(&bufferLength)), 0,
	)
	if windows.Errno(code) != windows.ERROR_INSUFFICIENT_BUFFER || count == 0 || bufferLength == 0 {
		return nil
	}

	names := make([]*uint16, count)
	buffer := make([]uint16, bufferLength)
	code, _, _ = getPackagesByPackageFamily.Call(
		uintptr(unsafe.Pointer(familyName)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&names[0])),
		uintptr(unsafe.Pointer(&bufferLength)), uintptr(unsafe.Pointer(&buffer[0])),
	)
	if code != 0 {
		return nil
	}
	result := make([]string, 0, count)
	for _, name := range names[:count] {
		result = append(result, windows.UTF16PtrToString(name))
	}
	runtime.KeepAlive(buffer)
	return result
}

func packagePath(fullName string) string {
	name, err := windows.UTF16PtrFromString(fullName)
	if err != nil {
		return ""
	}
	var length uint32
	code, _, _ := getPackagePathByFullName.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&length)), 0)
	if windows.Errno(code) != windows.ERROR_INSUFFICIENT_BUFFER || length == 0 {
		return ""
	}
	buffer := make([]uint16, length)
	code, _, _ = getPackagePathByFullName.Call(
		uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&length)), uintptr(unsafe.Pointer(&buffer[0])),
	)
	if code != 0 {
		return ""
	}
	return windows.UTF16ToString(buffer)
}
