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

package agent

import (
	"os"
	"path/filepath"
	"strings"
)

type homeDir struct {
	owner string
	path  string
}

type pathValues struct {
	platform    string
	home        string
	roaming     string
	local       string
	packageName string
}

func scanNpmInstallations(templates []string, values pathValues, owner string) []Installation {
	var result []Installation
	for _, spec := range agentSpecs {
		if spec.NpmPackage == "" {
			continue
		}
		values.packageName = spec.NpmPackage
		packageTemplates := append([]string(nil), templates...)
		for _, npmPath := range spec.NpmPaths {
			if npmPath.Platform == values.platform {
				packageTemplates = append(packageTemplates, npmPath.Template)
			}
		}
		for _, template := range packageTemplates {
			if strings.Contains(template, "{home}") && values.home == "" ||
				strings.Contains(template, "{roaming}") && values.roaming == "" ||
				strings.Contains(template, "{local}") && values.local == "" {
				continue
			}
			matches, _ := filepath.Glob(expandPath(template, values))
			for _, packageJSON := range matches {
				version, ok := readPackageVersion(packageJSON, spec.NpmPackage)
				if !ok {
					continue
				}
				packageOwner := owner
				if packageOwner == "" {
					packageOwner = pathOwner(packageJSON)
				}
				result = append(result, Installation{
					Name: spec.Name, Version: version, Path: filepath.Dir(packageJSON), InstallMethod: "npm", Owner: packageOwner,
				})
			}
		}
	}
	return result
}

func scanNativeInstallations(home homeDir, windows bool) []Installation {
	var result []Installation
	for _, spec := range agentSpecs {
		if spec.Native == nil {
			continue
		}
		launcherPath := spec.Native.UnixLauncher
		if windows {
			launcherPath = spec.Native.WindowsLauncher
		}
		launcher := filepath.Join(home.path, filepath.FromSlash(launcherPath))
		info, err := os.Stat(launcher)
		if err != nil || !info.Mode().IsRegular() || (!windows && info.Mode()&0o111 == 0) {
			continue
		}

		versionsDir := filepath.Join(home.path, filepath.FromSlash(spec.Native.VersionsDir))
		version := versionFromLauncher(launcher, versionsDir, info)
		result = append(result, Installation{
			Name: spec.Name, Version: version, Path: launcher, InstallMethod: "native", Owner: home.owner,
		})
	}
	return result
}

func versionFromLauncher(launcher, versionsDir string, launcherInfo os.FileInfo) string {
	if target, err := filepath.EvalSymlinks(launcher); err == nil {
		if relative, err := filepath.Rel(versionsDir, target); err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return strings.Split(relative, string(filepath.Separator))[0]
		}
	}
	if entries, err := os.ReadDir(versionsDir); err == nil {
		for _, entry := range entries {
			candidate, err := entry.Info()
			if err == nil && candidate.Mode().IsRegular() && os.SameFile(launcherInfo, candidate) {
				return entry.Name()
			}
		}
	}
	return ""
}

func scanGitInstallations(home homeDir, windows bool) []Installation {
	var result []Installation
	for _, spec := range agentSpecs {
		if spec.Git == nil || spec.NpmPackage == "" {
			continue
		}
		wrappers := spec.Git.UnixWrappers
		if windows {
			wrappers = spec.Git.WindowsWrappers
		}
		installed := false
		for _, wrapperPath := range wrappers {
			wrapper := filepath.Join(home.path, filepath.FromSlash(wrapperPath))
			if info, err := os.Stat(wrapper); err == nil && info.Mode().IsRegular() && (windows || info.Mode()&0o111 != 0) {
				installed = true
				break
			}
		}
		if !installed {
			continue
		}

		repo := filepath.Join(home.path, filepath.FromSlash(spec.Git.RepoDir))
		version, ok := readPackageVersion(filepath.Join(repo, "package.json"), spec.NpmPackage)
		if ok {
			result = append(result, Installation{
				Name: spec.Name, Version: version, Path: repo, InstallMethod: "git", Owner: home.owner,
			})
		}
	}
	return result
}

func scanHomebrewInstallations(prefix string) []Installation {
	var result []Installation
	for _, spec := range agentSpecs {
		for _, cask := range spec.HomebrewCasks {
			versions, _ := filepath.Glob(filepath.Join(prefix, "Caskroom", cask, "*"))
			for _, versionDir := range versions {
				executable := filepath.Join(versionDir, spec.Binary)
				if info, err := os.Stat(executable); err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
					continue
				}
				result = append(result, Installation{
					Name: spec.Name, Version: filepath.Base(versionDir), Path: executable, InstallMethod: "homebrew", Owner: pathOwner(executable),
				})
			}
		}
	}
	return result
}

func expandPath(template string, values pathValues) string {
	replacer := strings.NewReplacer(
		"{home}", values.home,
		"{roaming}", values.roaming,
		"{local}", values.local,
		"{package}", values.packageName,
	)
	return filepath.Clean(filepath.FromSlash(replacer.Replace(template)))
}
