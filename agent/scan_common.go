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
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// maxPackageJSONSize bounds how much of a candidate manifest we are willing to
// read, since the path it came from is only glob-verified.
const maxPackageJSONSize = 1024 * 1024

// scanNpmPatterns reports installations for every glob match whose package
// manifest confirms it really is the agent's npm package. ownerFor resolves the
// on-disk owner when owner is empty; it may be nil on platforms without one.
func scanNpmPatterns(fingerprint *compiledFingerprint, patterns []string, owner string, ownerFor func(string) string) []Installation {
	if fingerprint.NpmPackage == "" {
		return nil
	}

	var result []Installation
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, packageJSON := range matches {
			info, err := os.Stat(packageJSON)
			if err != nil || !info.Mode().IsRegular() || info.Size() > maxPackageJSONSize {
				continue
			}
			data, err := os.ReadFile(packageJSON)
			if err != nil {
				continue
			}
			var pkg struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}
			if json.Unmarshal(data, &pkg) != nil || pkg.Name != fingerprint.NpmPackage || pkg.Version == "" {
				continue
			}
			packageOwner := owner
			if packageOwner == "" && ownerFor != nil {
				packageOwner = ownerFor(packageJSON)
			}
			result = append(result, Installation{
				Name: fingerprint.DisplayName, Version: pkg.Version, Path: filepath.Dir(packageJSON),
				InstallMethod: "npm", Owner: packageOwner,
			})
		}
	}
	return result
}

// npmPackagePath renders a fingerprint's npm package name as a path fragment,
// so callers can compose it into a platform-specific glob.
func (f *compiledFingerprint) npmPackagePath() string {
	return filepath.FromSlash(f.NpmPackage)
}

// stampAgentId labels every installation appended since mark with the id of the
// fingerprint that produced it. Stamping once per fingerprint keeps the dozen
// construction sites spread across the platform scanners from each having to
// remember the field.
func stampAgentId(installations []Installation, mark int, agentId string) {
	for i := mark; i < len(installations); i++ {
		installations[i].AgentId = agentId
	}
}

// dedupeInstallations drops installations that resolve to the same executable
// and returns the rest ordered by owner then path. The result is never nil, so
// the API reports an empty list rather than null.
func dedupeInstallations(installations []Installation) []Installation {
	seen := map[string]bool{}
	result := make([]Installation, 0, len(installations))
	for _, installation := range installations {
		key := canonicalPath(installation.Path)
		if key == "" {
			continue
		}
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, installation)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Owner != result[j].Owner {
			return result[i].Owner < result[j].Owner
		}
		return result[i].Path < result[j].Path
	})
	return result
}

func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

// versionUnderDir returns the first path element of target relative to root, or
// "" when target does not live under root. Native installs keep their payload
// in <root>/<version>/, so that element is the version.
func versionUnderDir(root, target string) string {
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ""
	}
	return strings.Split(relative, string(filepath.Separator))[0]
}

// findExecutablePath picks the launcher out of a package manager's file
// listing, one path per line.
func findExecutablePath(files, execName string) string {
	for _, line := range strings.Split(files, "\n") {
		line = strings.TrimSpace(line)
		if filepath.Base(line) == execName {
			return line
		}
	}
	return ""
}
