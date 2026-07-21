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
	"strings"
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	stringadapter "github.com/casbin/casbin/v2/persist/string-adapter"
)

func blockedRecord() *Record {
	return &Record{
		Id:        "record-1",
		Agent:     "claude-code",
		Resource:  "127.0.0.1:3000#git_push",
		Intent:    "mcp.tool_call",
		PolicySet: "claude-code-strict",
		IsAllowed: false,
		Reason:    `mcp.tool_call on "127.0.0.1:3000#git_push" denied`,
		Feedback:  FeedbackAllow,
	}
}

// The point of a correction is that the call it was made about is decided the
// other way afterwards, so the test is the enforcement, not the text.
func TestLearnedRuleAllowsTheCorrectedCall(t *testing.T) {
	rule, err := LearnedRuleFromRecord(blockedRecord(), "alice")
	if err != nil {
		t.Fatalf("LearnedRuleFromRecord: %v", err)
	}

	set := BuildLearnedPolicySet("casbin", "alice", "Alice", "alice", []*LearnedRule{rule})

	m, err := model.NewModelFromString(string(set.Model))
	if err != nil {
		t.Fatalf("model: %v", err)
	}
	enforcer, err := casbin.NewEnforcer(m, stringadapter.NewAdapter(string(set.Policy)))
	if err != nil {
		t.Fatalf("enforcer: %v", err)
	}

	allowed, err := enforcer.Enforce("alice", "127.0.0.1:3000#git_push", "mcp.tool_call")
	if err != nil {
		t.Fatalf("enforce: %v", err)
	}
	if !allowed {
		t.Errorf("the corrected call is still denied by the rule learned from it")
	}
}

// A correction is a judgement about one call somebody looked at. If it widened
// into "every git_push anywhere", the operator would be granting permissions
// they never considered, which is exactly what a learned rule must not do.
func TestLearnedRuleDoesNotWiden(t *testing.T) {
	rule, err := LearnedRuleFromRecord(blockedRecord(), "alice")
	if err != nil {
		t.Fatalf("LearnedRuleFromRecord: %v", err)
	}

	set := BuildLearnedPolicySet("casbin", "alice", "Alice", "alice", []*LearnedRule{rule})
	m, _ := model.NewModelFromString(string(set.Model))
	enforcer, err := casbin.NewEnforcer(m, stringadapter.NewAdapter(string(set.Policy)))
	if err != nil {
		t.Fatalf("enforcer: %v", err)
	}

	for _, obj := range []string{"evil.example.com#git_push", "127.0.0.1:3000#git_push_all", "x127.0.0.1:3000#git_push"} {
		allowed, err := enforcer.Enforce("alice", obj, "mcp.tool_call")
		if err != nil {
			t.Fatalf("enforce %q: %v", obj, err)
		}
		if allowed {
			t.Errorf("the rule learned about %q also allowed %q", rule.Object, obj)
		}
	}
}

// A record with no Casbin triple was only logged, so there is nothing to learn.
func TestLearnedRuleNeedsAnOperation(t *testing.T) {
	record := blockedRecord()
	record.Resource, record.Intent = "", ""
	if _, err := LearnedRuleFromRecord(record, "alice"); err == nil {
		t.Errorf("expected an error for a record that carries no operation")
	}
}

// Correcting the same record twice replaces its rule; withdrawing the feedback
// removes it. Otherwise a person changing their mind would leave both verdicts
// in the policy at once.
func TestUpsertLearnedRuleReplacesAndRemoves(t *testing.T) {
	first, _ := LearnedRuleFromRecord(blockedRecord(), "alice")

	tightened := blockedRecord()
	tightened.Feedback = FeedbackDeny
	second, _ := LearnedRuleFromRecord(tightened, "alice")

	rules := UpsertLearnedRule([]*LearnedRule{}, "record-1", first)
	rules = UpsertLearnedRule(rules, "record-1", second)
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule after re-correcting the same record, got %d", len(rules))
	}
	if rules[0].Effect != "deny" {
		t.Errorf("expected the newer correction to win, got effect %q", rules[0].Effect)
	}

	if rules := UpsertLearnedRule(rules, "record-1", nil); len(rules) != 0 {
		t.Errorf("expected withdrawing the feedback to unlearn the rule, got %d rules", len(rules))
	}
}

// The rules survive a trip through a Casdoor user property, and a property that
// somebody mangled by hand degrades to "nothing learned" rather than breaking
// the page that would let them fix it.
func TestParseLearnedRulesRoundTrip(t *testing.T) {
	rule, _ := LearnedRuleFromRecord(blockedRecord(), "alice")

	value, err := MarshalLearnedRules([]*LearnedRule{rule})
	if err != nil {
		t.Fatalf("MarshalLearnedRules: %v", err)
	}
	parsed := ParseLearnedRules(value)
	if len(parsed) != 1 || parsed[0].Object != rule.Object || parsed[0].Effect != rule.Effect {
		t.Errorf("the rule did not survive the round trip: %+v", parsed)
	}

	for _, broken := range []string{"", "   ", "not json", `[{"object": "x"}]`} {
		if rules := ParseLearnedRules(broken); len(rules) != 0 {
			t.Errorf("expected no rules from %q, got %d", broken, len(rules))
		}
	}
}

// An empty set still has to be a policy the enforcer can load, since the fusion
// page builds one before anybody has corrected anything.
func TestEmptyLearnedPolicySetIsLoadable(t *testing.T) {
	set := BuildLearnedPolicySet("casbin", "alice", "Alice", "alice", []*LearnedRule{})
	if strings.TrimSpace(string(set.Request)) != "" {
		t.Errorf("expected no example requests, got %q", set.Request)
	}

	m, _ := model.NewModelFromString(string(set.Model))
	if _, err := casbin.NewEnforcer(m, stringadapter.NewAdapter(string(set.Policy))); err != nil {
		t.Fatalf("an empty learned set failed to load: %v", err)
	}
}
