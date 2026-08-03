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

//go:build linux || darwin

package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestScanDetectsRunningOpenAgent is a local, opt-in check that a real process
// named "openagent" is discovered by Scan() on Linux and macOS. It builds a tiny
// sleeper, names it "openagent", runs it, and looks for it in the scan. Run with:
//
//	AIGUARD_PROCESS_SCAN_E2E=1 go test -v -run TestScanDetectsRunningOpenAgent ./agent/
func TestScanDetectsRunningOpenAgent(t *testing.T) {
	if os.Getenv("AIGUARD_PROCESS_SCAN_E2E") == "" {
		t.Skip("set AIGUARD_PROCESS_SCAN_E2E=1 to run the local process-scan check")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	if err := os.WriteFile(source, []byte("package main\n\nimport \"time\"\n\nfunc main() { time.Sleep(60 * time.Second) }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "openagent")
	if out, err := exec.Command("go", "build", "-o", binary, source).CombinedOutput(); err != nil {
		t.Fatalf("could not build the dummy openagent: %v\n%s", err, out)
	}

	process := exec.Command(binary)
	if err := process.Start(); err != nil {
		t.Fatalf("could not start the dummy openagent: %v", err)
	}
	defer func() { _ = process.Process.Kill() }()
	time.Sleep(700 * time.Millisecond) // let it show up in the process table

	installations := Scan()
	t.Logf("Scan() returned %d installation(s):", len(installations))
	found := false
	for _, installation := range installations {
		t.Logf("  - %-14s | method=%-9s | owner=%-8s | %s",
			installation.Name, installation.InstallMethod, installation.Owner, installation.Path)
		if installation.Path == binary && installation.AgentId == "openagent" && installation.InstallMethod == "process" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the running openagent at %s was NOT detected by Scan()", binary)
	}
	t.Logf("OK: detected the running openagent at %s", binary)
}
