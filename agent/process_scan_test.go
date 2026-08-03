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

import "testing"

// TestInstallationsFromProcesses pins the mapping from running processes to
// installations: an agent binary at any path is recognized (including a source
// build), the owner is carried through so the result can be patched, duplicate
// paths collapse to one, and unrelated executables are ignored.
func TestInstallationsFromProcesses(t *testing.T) {
	processes := []processInfo{
		{Path: "/opt/openagent/openagent", Owner: "alice"},        // source build in a custom dir
		{Path: "/home/bob/src/openagent/openagent", Owner: "bob"}, // source checkout
		{Path: "/opt/openagent/openagent", Owner: "alice"},        // duplicate of the first
		{Path: "/usr/bin/node", Owner: "root"},                    // unrelated
		{Path: "", Owner: "root"},                                 // empty
	}

	got := installationsFromProcesses(processes)
	if len(got) != 2 {
		t.Fatalf("got %d installations, want 2 (dedup + drop unrelated): %+v", len(got), got)
	}
	for _, installation := range got {
		if installation.AgentId != "openagent" {
			t.Errorf("agentId = %q, want openagent", installation.AgentId)
		}
		if installation.Name != "OpenAgent" {
			t.Errorf("name = %q, want OpenAgent", installation.Name)
		}
		if installation.InstallMethod != "process" {
			t.Errorf("installMethod = %q, want process", installation.InstallMethod)
		}
	}
	if got[0].Path != "/opt/openagent/openagent" || got[0].Owner != "alice" {
		t.Errorf("first: path=%q owner=%q", got[0].Path, got[0].Owner)
	}
	if got[1].Path != "/home/bob/src/openagent/openagent" || got[1].Owner != "bob" {
		t.Errorf("second: path=%q owner=%q", got[1].Path, got[1].Owner)
	}
}

func TestDisplayNameForID(t *testing.T) {
	if got := displayNameForID("openagent"); got != "OpenAgent" {
		t.Errorf("displayNameForID(openagent) = %q, want OpenAgent", got)
	}
	if got := displayNameForID("no-such-agent"); got != "no-such-agent" {
		t.Errorf("unknown id should fall back to itself, got %q", got)
	}
}
