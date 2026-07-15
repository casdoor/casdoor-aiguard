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
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var linuxSystemNpmTemplates = []string{
	"/usr/local/lib/node_modules/{package}/package.json",
	"/usr/lib/node_modules/{package}/package.json",
}

func scanPlatform() []Installation {
	homes := readHomes("/etc/passwd")
	var installations []Installation
	for _, home := range homes {
		values := pathValues{platform: "unix", home: home.path}
		installations = append(installations, scanNpmInstallations(unixUserNpmTemplates, values, home.owner)...)
		installations = append(installations, scanNativeInstallations(home, false)...)
		installations = append(installations, scanGitInstallations(home, false)...)
	}
	installations = append(installations, scanNpmInstallations(linuxSystemNpmTemplates, pathValues{platform: "unix"}, "")...)
	installations = append(installations, scanLinuxPackages()...)
	for _, prefix := range []string{"/home/linuxbrew/.linuxbrew", "/opt/homebrew", "/usr/local"} {
		installations = append(installations, scanHomebrewInstallations(prefix)...)
	}
	return installations
}

func readHomes(passwdPath string) []homeDir {
	data, err := os.ReadFile(passwdPath)
	if err != nil {
		return nil
	}

	seen := map[string]bool{}
	var homes []homeDir
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 6 || !filepath.IsAbs(fields[5]) || seen[fields[5]] {
			continue
		}
		info, err := os.Stat(fields[5])
		if err != nil || !info.IsDir() {
			continue
		}
		seen[fields[5]] = true
		homes = append(homes, homeDir{owner: fields[0], path: fields[5]})
	}
	return homes
}

func scanLinuxPackages() []Installation {
	var result []Installation
	for _, spec := range agentSpecs {
		if spec.LinuxPackage == "" {
			continue
		}
		if version, ok := commandOutput("dpkg-query", "-W", "-f=${Version}", spec.LinuxPackage); ok {
			files, _ := commandOutput("dpkg", "-L", spec.LinuxPackage)
			result = append(result, linuxPackageInstallation(spec, "apt", version, files))
		}
		if version, ok := commandOutput("rpm", "-q", "--qf", "%{VERSION}-%{RELEASE}", spec.LinuxPackage); ok {
			files, _ := commandOutput("rpm", "-ql", spec.LinuxPackage)
			result = append(result, linuxPackageInstallation(spec, "rpm", version, files))
		}
		if installed, ok := commandOutput("apk", "info", "-e", spec.LinuxPackage); ok && strings.TrimSpace(installed) != "" {
			version, _ := commandOutput("apk", "info", "-v", spec.LinuxPackage)
			files, _ := commandOutput("apk", "info", "-L", spec.LinuxPackage)
			version = strings.TrimPrefix(strings.TrimSpace(version), spec.LinuxPackage+"-")
			result = append(result, linuxPackageInstallation(spec, "apk", version, files))
		}
	}
	return result
}

func linuxPackageInstallation(spec agentSpec, method, version, files string) Installation {
	path := findBinaryPath(files, spec.Binary)
	if path == "" {
		path = filepath.Join("/usr/bin", spec.Binary)
	}
	return Installation{
		Name: spec.Name, Version: strings.TrimSpace(version), Path: path, InstallMethod: method, Owner: "root",
	}
}

func findBinaryPath(files, binary string) string {
	for _, line := range strings.Split(files, "\n") {
		line = strings.TrimSpace(line)
		if filepath.Base(line) == binary {
			return line
		}
	}
	return ""
}

func commandOutput(name string, args ...string) (string, bool) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	return commandOutputPath(path, args...)
}

func commandOutputPath(path string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, args...).Output()
	return string(out), err == nil
}
