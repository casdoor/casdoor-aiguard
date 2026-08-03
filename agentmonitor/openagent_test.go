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

package agentmonitor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/casdoor/casdoor-aiguard/object"
)

// TestOpenAgentMonitorTailsAppendedLines checks the whole audit pipeline: enabling
// a claim skips history already on disk, and only lines appended afterwards become
// records, mapped onto aiguard's taxonomy (builtin tool, MCP tool, guard deny).
func TestOpenAgentMonitorTailsAppendedLines(t *testing.T) {
	tmp := t.TempDir()
	auditDir := filepath.Join(tmp, "audit")
	if err := os.MkdirAll(auditDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionFile := filepath.Join(auditDir, "sess1.jsonl")

	// A line written before the claim must be skipped, not replayed.
	writeLine(t, sessionFile, `{"type":"tool_call","tool":"old","sessionId":"sess1","outcome":"success"}`)

	var records []*object.Record
	m := newOpenAgentMonitorManager(filepath.Join(tmp, "state.json"), func(r *object.Record) {
		records = append(records, r)
	})
	if err := m.enable(openAgentClaim{Path: filepath.Join(tmp, "openagent"), Owner: "alice", AuditDir: auditDir}); err != nil {
		t.Fatalf("enable: %v", err)
	}

	writeLine(t, sessionFile, `{"type":"tool_call","tool":"read_file","model":"m1","argumentsLength":10,"effect":"allow","outcome":"success","durationMs":5,"sessionId":"sess1"}`)
	writeLine(t, sessionFile, `{"type":"tool_call","tool":"search","server":"web","outcome":"success","sessionId":"sess1"}`)
	writeLine(t, sessionFile, `{"type":"tool_call","tool":"delete_all","effect":"deny","reason":"blocked by rule","sessionId":"sess1"}`)

	m.poll()

	if len(records) != 3 {
		t.Fatalf("got %d records, want 3 (history must be skipped): %+v", len(records), records)
	}

	builtin := records[0]
	if builtin.EventType != "tool" || builtin.Action != "call" || builtin.Outcome != "success" {
		t.Errorf("builtin tool: got %s/%s/%s", builtin.EventType, builtin.Action, builtin.Outcome)
	}
	if builtin.ToolName != "read_file" || builtin.Model != "m1" || builtin.DurationMs != 5 {
		t.Errorf("builtin fields: tool=%q model=%q dur=%d", builtin.ToolName, builtin.Model, builtin.DurationMs)
	}
	if builtin.Agent != "openagent" || builtin.User != "alice" || builtin.SessionKey != "sess1" {
		t.Errorf("base fields: agent=%q user=%q session=%q", builtin.Agent, builtin.User, builtin.SessionKey)
	}

	mcp := records[1]
	if mcp.EventType != "mcp" || mcp.McpServer != "web" || mcp.McpTool != "search" || mcp.ToolName != "mcp__web__search" {
		t.Errorf("mcp mapping: type=%s server=%q tool=%q name=%q", mcp.EventType, mcp.McpServer, mcp.McpTool, mcp.ToolName)
	}

	denied := records[2]
	if denied.Outcome != "denied" {
		t.Errorf("deny effect should map to denied outcome, got %q", denied.Outcome)
	}
	if denied.Detail != "blocked by rule" {
		t.Errorf("deny reason should land in Detail, got %q", denied.Detail)
	}
}

// TestOpenAgentMonitorReloadsCursor checks that a restart resumes from the saved
// byte offset instead of replaying the whole file.
func TestOpenAgentMonitorReloadsCursor(t *testing.T) {
	tmp := t.TempDir()
	auditDir := filepath.Join(tmp, "audit")
	if err := os.MkdirAll(auditDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionFile := filepath.Join(auditDir, "sess1.jsonl")
	statePath := filepath.Join(tmp, "state.json")
	claim := openAgentClaim{Path: filepath.Join(tmp, "openagent"), Owner: "alice", AuditDir: auditDir}

	first := newOpenAgentMonitorManager(statePath, func(*object.Record) {})
	if err := first.enable(claim); err != nil {
		t.Fatalf("enable: %v", err)
	}
	writeLine(t, sessionFile, `{"type":"tool_call","tool":"a","sessionId":"sess1","outcome":"success"}`)
	first.poll()

	var second []*object.Record
	reloaded := newOpenAgentMonitorManager(statePath, func(r *object.Record) {
		second = append(second, r)
	})
	if err := reloaded.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer reloaded.stopMonitor()

	writeLine(t, sessionFile, `{"type":"tool_call","tool":"b","sessionId":"sess1","outcome":"success"}`)
	reloaded.poll()

	if len(second) != 1 || second[0].ToolName != "b" {
		t.Fatalf("reload should resume from saved offset and see only %q, got %+v", "b", second)
	}
}

func writeLine(t *testing.T, path, line string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
}
