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

package object

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A minimal OpenClaw set: reads are allowed, deletes are denied. It is the same
// shape as the shipped sets under data/policyhub, small enough to reason about.
const testOpenclawSet = `{
  "displayName": "OpenClaw test",
  "agent": "OpenClaw",
  "os": "Windows",
  "model": [
    "[request_definition]",
    "r = sub, obj, act",
    "[policy_definition]",
    "p = sub, obj, act, eft",
    "[policy_effect]",
    "e = some(where (p_eft == allow)) && !some(where (p_eft == deny))",
    "[matchers]",
    "m = r.sub == p.sub && regexMatch(r.obj, p.obj) && regexMatch(r.act, p.act)"
  ],
  "policy": [
    "p, openclaw, ^(127\\.0\\.0\\.1|localhost)(:\\d+)?#(read_file|write_file)$, mcp\\.tool_call, allow",
    "p, openclaw, .*#(delete_file|git_push)$, mcp\\.tool_call, deny"
  ],
  "request": ["openclaw, 127.0.0.1#read_file, mcp.tool_call"]
}`

// TestEnforceForAgent checks the point that actually blocks an agent: an enabled
// OpenClaw set must deny a delete and allow a read, and must ignore an agent its
// rules are not written for.
func TestEnforceForAgent(t *testing.T) {
	dir := t.TempDir()
	// GetConfigString reads env first, so this reroutes both the policy hub the
	// enforcer reads sets from and the file the enable state persists to.
	t.Setenv("policyHubDir", dir)
	t.Setenv("policyFile", filepath.Join(dir, "policy.yaml"))

	if err := os.WriteFile(filepath.Join(dir, "openclaw-test.json"), []byte(testOpenclawSet), 0o644); err != nil {
		t.Fatalf("write set: %v", err)
	}

	if err := SetPolicySetEnabled("openclaw-test", true); err != nil {
		t.Fatalf("enable set: %v", err)
	}
	t.Cleanup(func() { _ = SetPolicySetEnabled("openclaw-test", false) })

	deny := EnforceForAgent("openclaw", "127.0.0.1#delete_file", "mcp.tool_call")
	if deny.Allowed {
		t.Errorf("delete_file: got allow, want deny")
	}
	if deny.PolicySet != "openclaw-test" {
		t.Errorf("delete_file: deny attributed to %q, want openclaw-test", deny.PolicySet)
	}

	// The Records page shows this line as the block reason, so it has to name
	// both the set that refused and what it refused.
	reason := deny.Reason("127.0.0.1#delete_file", "mcp.tool_call")
	if !strings.Contains(reason, "openclaw-test") || !strings.Contains(reason, "delete_file") {
		t.Errorf("deny reason = %q, want it to name the set and the operation", reason)
	}

	if allow := EnforceForAgent("openclaw", "127.0.0.1#read_file", "mcp.tool_call"); !allow.Allowed {
		t.Errorf("read_file: got deny (%q), want allow", allow.PolicySet)
	}

	// A set only guards its own agent: the same denied call from a different
	// agent is not in scope, so it passes through.
	if other := EnforceForAgent("claude-desktop", "127.0.0.1#delete_file", "mcp.tool_call"); !other.Allowed {
		t.Errorf("delete_file as claude-desktop: got deny, want allow (out of scope)")
	}
}

func TestSubjectOf(t *testing.T) {
	policy := "# comment\np, openclaw, ^x$, mcp.tool_call, allow\np, openclaw, .*, payment, deny"
	if got := subjectOf(policy); got != "openclaw" {
		t.Errorf("subjectOf = %q, want openclaw", got)
	}
	if got := subjectOf("# only a comment\n\n"); got != "" {
		t.Errorf("subjectOf with no rule = %q, want empty", got)
	}
}
