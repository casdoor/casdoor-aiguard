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
	"sync"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	stringadapter "github.com/casbin/casbin/v2/persist/string-adapter"
)

// This is the server-side counterpart of web/src/utils/casbin.js: it runs the
// exact same Casbin model and policy, but as the point that actually blocks an
// agent rather than as a preview. The Web UI shows what a set would decide; this
// makes an enabled set decide for real.
//
// One triple is enforced per intercepted call, matching the vocabulary every
// shipped set documents:
//   sub = the agent (e.g. "openclaw"), obj = the destination host ("host#tool"
//   for an MCP tools/call), act = the intent category ("mcp.tool_call",
//   "llm.chat", ...). See data/policyhub/*.json.

// Decision is the outcome of enforcing every enabled set against one operation.
// The zero value allows: a call no enabled set denies is allowed through.
type Decision struct {
	Allowed bool `json:"allowed"`
	// PolicySet and Rule identify the deny that blocked the call, so a surprising
	// block is traceable back to a single rule the way the Web UI's preview is.
	PolicySet string   `json:"policySet,omitempty"`
	Rule      []string `json:"rule,omitempty"`
}

// compiledSet caches one set's enforcer alongside the model and policy text it
// was built from, so it is rebuilt only when the set's file actually changes.
type compiledSet struct {
	model    string
	policy   string
	subject  string
	enforcer *casbin.Enforcer
}

var (
	enforcerCache   = map[string]*compiledSet{}
	enforcerCacheMu sync.Mutex
)

// subjectOf returns the subject the set's policy is written for: the "sub"
// column of its first "p," rule. Every rule in a shipped set shares one subject
// (the agent it guards), so the first one identifies the whole set. It is empty
// when the policy declares no rule.
func subjectOf(policy string) string {
	for _, line := range strings.Split(policy, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "p,") && !strings.HasPrefix(line, "p ,") {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 2 {
			continue
		}
		return strings.TrimSpace(fields[1])
	}
	return ""
}

// enforcerFor returns a cached enforcer for the set, rebuilding it when the set's
// model or policy text has changed since it was last compiled.
func enforcerFor(set *PolicySet) (*compiledSet, error) {
	modelText := string(set.Model)
	policyText := string(set.Policy)

	enforcerCacheMu.Lock()
	defer enforcerCacheMu.Unlock()

	if cached, ok := enforcerCache[set.Name]; ok && cached.model == modelText && cached.policy == policyText {
		return cached, nil
	}

	m, err := model.NewModelFromString(modelText)
	if err != nil {
		return nil, err
	}
	enforcer, err := casbin.NewEnforcer(m, stringadapter.NewAdapter(policyText))
	if err != nil {
		return nil, err
	}

	compiled := &compiledSet{
		model:    modelText,
		policy:   policyText,
		subject:  subjectOf(policyText),
		enforcer: enforcer,
	}
	enforcerCache[set.Name] = compiled
	return compiled, nil
}

// EnforceForAgent evaluates one operation against every enabled set written for
// the given agent. It returns the first deny encountered (named, with the rule
// that decided it) and otherwise allows: no enabled set that applies means the
// operation was never in scope of a guard, so it passes through.
//
// A set that fails to compile is skipped rather than treated as a deny: a broken
// policy file must not silently block an agent. The controller surfaces such a
// set separately when it is toggled on.
func EnforceForAgent(agentId, obj, act string) Decision {
	if obj == "" || act == "" {
		return Decision{Allowed: true}
	}

	for _, name := range EnabledPolicySetNames() {
		set, err := GetPolicySet(name)
		if err != nil || set == nil {
			continue
		}
		compiled, err := enforcerFor(set)
		if err != nil || compiled.subject == "" {
			continue
		}
		// A set only guards its own agent; the subject is that agent's id.
		if !strings.EqualFold(compiled.subject, agentId) {
			continue
		}

		allowed, rule, err := compiled.enforcer.EnforceEx(compiled.subject, obj, act)
		if err != nil {
			continue
		}
		if !allowed {
			return Decision{Allowed: false, PolicySet: name, Rule: rule}
		}
	}
	return Decision{Allowed: true}
}

// invalidateEnforcer drops a set's cached enforcer, so the next enforcement
// rebuilds it. Called when a set is toggled, in case its file changed while it
// was disabled.
func invalidateEnforcer(name string) {
	enforcerCacheMu.Lock()
	delete(enforcerCache, name)
	enforcerCacheMu.Unlock()
}
