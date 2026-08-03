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

// Process discovery finds an agent that is running from a path no install-layout
// scan would look at - most importantly a build from source. The filesystem
// scanners only know fixed install layouts and never walk the disk, so a source
// checkout is invisible to them; enumerating running processes and matching each
// one's executable by the same rules IdentifyExecutable already uses closes that
// gap without executing anything or walking arbitrary directories.
//
// This runs on every supported OS (Linux, macOS and Windows), so the Agents
// inventory is consistent across them: a host must not appear to have no agent
// just because the reader happens to sit on a different platform. On Linux the
// interception path also resolves /proc/<pid>/exe, but only for a connection it
// catches - that attributes traffic, it does not populate the inventory, so a
// running source build still needs this to show up on the Agents page.

// processInfo is one running process reduced to what discovery needs: the
// executable path, and the account it runs as so a discovered install can be
// patched (the patcher needs an owner).
type processInfo struct {
	Path  string
	Owner string
}

// scanRunningProcesses reports one installation per distinct running agent
// executable, discovered by walking the process table rather than the disk.
func scanRunningProcesses() []Installation {
	return installationsFromProcesses(enumerateProcesses())
}

// installationsFromProcesses turns running processes into installations, keeping
// only the ones that match a known agent and collapsing duplicate paths (the
// same binary running many times is one installation).
func installationsFromProcesses(processes []processInfo) []Installation {
	seen := map[string]bool{}
	var result []Installation
	for _, process := range processes {
		if process.Path == "" || seen[process.Path] {
			continue
		}
		seen[process.Path] = true

		agentId := IdentifyExecutable(process.Path)
		if agentId == "" {
			continue
		}
		result = append(result, Installation{
			AgentId:       agentId,
			Name:          displayNameForID(agentId),
			Path:          process.Path,
			InstallMethod: "process",
			Owner:         process.Owner,
		})
	}
	return result
}

// displayNameForID returns the human-readable name for an agent id, falling back
// to the id itself if no fingerprint carries a display name.
func displayNameForID(agentId string) string {
	for _, fingerprint := range compiledFingerprints {
		if fingerprint.ID == agentId {
			return fingerprint.DisplayName
		}
	}
	return agentId
}
