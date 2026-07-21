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
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// A hand-written policy set is a guess about what an agent will do; the Records
// page is the record of what it actually did. The self-learned policy set is
// what closes the gap between the two: whenever an operator looks at a blocked
// record and says "that should have been allowed", that judgement is turned
// into a Casbin rule about exactly the call that was blocked.
//
// The rules are stored on the Casdoor user, in one more "properties" entry
// beside the digital employee's three, so what a person taught aiguard on one
// machine follows them to the next. They are kept as JSON rather than as policy
// text because a learned rule remembers where it came from - which record, which
// agent, which block it overturns - and that provenance is the whole point: a
// generated rule nobody can trace back is a rule nobody will trust.
const LearnedRulesProperty = "aiguardLearnedRules"

// LearnedRule is one operator correction, kept with everything needed to render
// it as a Casbin rule and to explain why it exists.
type LearnedRule struct {
	// Id is the record the correction was made on, so re-correcting the same
	// record replaces its rule instead of piling up a second one.
	Id string `json:"id"`

	// The Casbin rule itself. Object and Action are the literal values observed
	// on the record; ToPolicyLine turns them into an anchored pattern, because
	// the model matches them with regexMatch.
	Subject string `json:"subject"`
	Object  string `json:"object"`
	Action  string `json:"action"`
	Effect  string `json:"effect"`

	// Where it came from: the agent that made the call, the verdict being
	// overturned, and when the person said so.
	Agent       string `json:"agent"`
	PolicySet   string `json:"policySet,omitempty"`
	WasAllowed  bool   `json:"wasAllowed"`
	Reason      string `json:"reason,omitempty"`
	CreatedTime string `json:"createdTime"`
}

// LearnedPolicySet is the learned rules seen as a policy set: the same three
// text blocks a Policy Hub set or a digital employee is made of, so the editors,
// the enforcement preview and policy fusion all read it with the code they
// already have. The rules stay alongside as the provenance the page shows.
type LearnedPolicySet struct {
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Subject     string `json:"subject"`

	Model   Text `json:"model"`
	Policy  Text `json:"policy"`
	Request Text `json:"request"`

	Rules []*LearnedRule `json:"rules"`
}

// regexLiteral turns an observed value into a pattern that matches that value
// and nothing else. A learned rule is a statement about one call the operator
// actually looked at, so it is deliberately not generalized: "127.0.0.1:3000#git_push
// was fine" must not quietly become "every git_push anywhere is fine".
func regexLiteral(value string) string {
	return "^" + regexp.QuoteMeta(value) + "$"
}

// ToPolicyLine renders the rule the way it appears in the generated policy.
func (r *LearnedRule) ToPolicyLine() string {
	return fmt.Sprintf("p, %s, %s, %s, %s", r.Subject, regexLiteral(r.Object), regexLiteral(r.Action), r.Effect)
}

// ToRequestLine renders the call the rule was learned from, so the enforcement
// preview below the policy always has the corrected call in it.
func (r *LearnedRule) ToRequestLine() string {
	return fmt.Sprintf("%s, %s, %s", r.Subject, r.Object, r.Action)
}

// LearnedRuleFromRecord builds the rule one corrected record teaches. The
// effect is the operator's feedback, not the original verdict: they are saying
// what should have happened, and that is exactly what a rule says.
func LearnedRuleFromRecord(record *Record, subject string) (*LearnedRule, error) {
	if !record.IsCorrectable() {
		return nil, fmt.Errorf("the record %q carries no operation to learn from", record.Id)
	}

	effect := "allow"
	if record.Feedback == FeedbackDeny {
		effect = "deny"
	}

	return &LearnedRule{
		Id:          record.Id,
		Subject:     subject,
		Object:      record.Resource,
		Action:      record.Intent,
		Effect:      effect,
		Agent:       record.Agent,
		PolicySet:   record.PolicySet,
		WasAllowed:  record.IsAllowed,
		Reason:      record.Reason,
		CreatedTime: time.Now().Format(time.RFC3339),
	}, nil
}

// ParseLearnedRules reads the rules back from a Casdoor user property. A
// property that cannot be parsed is treated as no rules rather than as an
// error: an unreadable learning history must not lock a person out of the page
// that would let them rebuild it.
func ParseLearnedRules(value string) []*LearnedRule {
	rules := []*LearnedRule{}
	if strings.TrimSpace(value) == "" {
		return rules
	}
	if err := json.Unmarshal([]byte(value), &rules); err != nil {
		return []*LearnedRule{}
	}

	// Drop anything a hand-edited property left incomplete, so the generated
	// policy never contains a half-written rule.
	kept := rules[:0]
	for _, rule := range rules {
		if rule != nil && rule.Object != "" && rule.Action != "" && rule.Subject != "" {
			if rule.Effect != "deny" {
				rule.Effect = "allow"
			}
			kept = append(kept, rule)
		}
	}
	return kept
}

// MarshalLearnedRules serializes the rules for storage, newest first so the
// property reads the way the page does.
func MarshalLearnedRules(rules []*LearnedRule) (string, error) {
	sorted := make([]*LearnedRule, len(rules))
	copy(sorted, rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].CreatedTime > sorted[j].CreatedTime
	})

	data, err := json.Marshal(sorted)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// UpsertLearnedRule adds a rule or replaces the one already learned from the
// same record, and returns the new list. A nil rule removes the record's rule
// instead, which is how withdrawing feedback unlearns what it taught.
func UpsertLearnedRule(rules []*LearnedRule, recordId string, rule *LearnedRule) []*LearnedRule {
	result := make([]*LearnedRule, 0, len(rules)+1)
	for _, existing := range rules {
		if existing.Id != recordId {
			result = append(result, existing)
		}
	}
	if rule != nil {
		result = append(result, rule)
	}
	return result
}

// learnedPolicyHeader explains the generated policy inside the policy itself,
// so the text stays self-describing wherever it is read - including once it has
// been fused into a bigger set.
const learnedPolicyHeader = `# Self-learned policy set - generated, not written.
# Every rule below came from a record on the Records page that a person
# corrected: aiguard ruled one way, they said it should have gone the other
# way, and the rule states what they said. Each one is anchored to the exact
# destination and intent that was observed, so a correction never widens into
# a permission nobody granted.
#
# Withdraw a correction on the Records page and its rule disappears from here.`

// BuildLearnedPolicySet renders the rules as the policy set the UI works with.
// The model is the digital employee's, because a learned rule is about the same
// person and has to fuse with their set without reconciling two request shapes.
func BuildLearnedPolicySet(owner string, name string, displayName string, subject string, rules []*LearnedRule) *LearnedPolicySet {
	allows, denies := []string{}, []string{}
	requests := []string{}
	for _, rule := range rules {
		if rule.Effect == "deny" {
			denies = append(denies, rule.ToPolicyLine())
		} else {
			allows = append(allows, rule.ToPolicyLine())
		}
		requests = append(requests, rule.ToRequestLine())
	}

	sections := []string{learnedPolicyHeader}
	if len(allows) > 0 {
		sections = append(sections, "", "# --- Blocked calls a person said should have been allowed. ---", strings.Join(allows, "\n"))
	}
	if len(denies) > 0 {
		sections = append(sections, "", "# --- Allowed calls a person said should have been blocked. ---", strings.Join(denies, "\n"))
	}
	if len(allows) == 0 && len(denies) == 0 {
		sections = append(sections, "", "# Nothing has been learned yet.")
	}

	model, _, _ := DefaultEmployeePolicySet(subject)

	return &LearnedPolicySet{
		Owner:       owner,
		Name:        name,
		DisplayName: displayName,
		Subject:     subject,
		Model:       model,
		Policy:      Text(strings.Join(sections, "\n")),
		Request:     Text(strings.Join(requests, "\n")),
		Rules:       rules,
	}
}
