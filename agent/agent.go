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

// Package agent discovers AI agents installed on the host and identifies
// running agent executables. Discovery is intentionally bounded to known,
// verifiable installation layouts rather than walking the whole filesystem.
package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Installation describes one AI agent installation found on the host.
type Installation struct {
	Name          string `json:"name"`
	Version       string `json:"version,omitempty"`
	Path          string `json:"path"`
	InstallMethod string `json:"installMethod"`
	Owner         string `json:"owner"`
}

// Scan returns supported AI agent installations found in known platform
// layouts. Results are de-duplicated by their resolved path.
func Scan() []Installation {
	return normalizeInstallations(scanPlatform())
}

func normalizeInstallations(installations []Installation) []Installation {
	seen := map[string]bool{}
	result := installations[:0]
	for _, installation := range installations {
		key := canonicalPath(installation.Path)
		if key == "" || seen[key] {
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

func readPackageVersion(packageJSON, packageName string) (string, bool) {
	info, err := os.Stat(packageJSON)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1024*1024 {
		return "", false
	}
	data, err := os.ReadFile(packageJSON)
	if err != nil {
		return "", false
	}
	var pkg struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if json.Unmarshal(data, &pkg) != nil || pkg.Name != packageName || pkg.Version == "" {
		return "", false
	}
	return pkg.Version, true
}

// IdentifyExecutable returns the agent name for a known executable layout.
// On Linux, the caller should pass the resolved /proc/<pid>/exe path.
func IdentifyExecutable(path string) string {
	path = strings.ToLower(filepath.ToSlash(filepath.Clean(path)))
	for _, spec := range agentSpecs {
		if spec.NpmPackage != "" && strings.Contains(path, "/node_modules/"+strings.ToLower(spec.NpmPackage)+"/") {
			return spec.ID
		}
		for _, exact := range spec.ExecutablePaths {
			if path == strings.ToLower(filepath.ToSlash(exact)) {
				return spec.ID
			}
		}
		for _, fingerprint := range spec.ExecutableFingerprints {
			if fingerprint.Suffix != "" && !strings.HasSuffix(path, fingerprint.Suffix) {
				continue
			}
			matched := true
			for _, part := range fingerprint.Contains {
				if !strings.Contains(path, part) {
					matched = false
					break
				}
			}
			if matched {
				return spec.ID
			}
		}
	}
	return ""
}
