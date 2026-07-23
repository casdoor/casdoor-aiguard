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

// Package patch instruments the AI agents the agent package discovers, so a
// patched agent streams its behaviour log to aiguard in real time (see
// object.Record).
//
// Instrumentation is selected by extension shape: hook directories, command
// hooks or JSON MCP registrations. Compatible agents share an implementation
// and differ only by profile data. File-based patchers can use the backup
// journal in state.go; patchers that edit shared settings remove only the
// entries they own.
//
// An agent with a new extension shape implements Patcher once; subsequent
// compatible agents add profiles.
package patch

import (
	"errors"
	"fmt"
	"sort"
)

// ErrNotSupported is what a Patcher returns when aiguard knows the agent but
// cannot instrument it yet. It is not a failure of the patch attempt: the host
// is untouched.
var ErrNotSupported = errors.New("patching this agent is not supported yet")

// Target names the one installation to patch. It is what the Web UI posts back
// from a row of the agents table, so it carries no more than that row knows.
type Target struct {
	AgentId string `json:"agentId"`
	Path    string `json:"path"`
	Owner   string `json:"owner"`
}

func (t Target) String() string {
	return fmt.Sprintf("%s(%s)", t.AgentId, t.Path)
}

// Status is what the agents table shows for one installation. Supported is a
// property of the agent (is a working Patcher registered?); Patched is a
// property of this installation right now, probed from the host rather than
// trusted from aiguard's own bookkeeping.
type Status struct {
	Supported bool   `json:"supported"`
	Patched   bool   `json:"patched"`
	Detail    string `json:"detail,omitempty"`
}

// Patcher instruments one kind of agent. Implementations must be idempotent:
// patching an already-patched target and unpatching an unpatched one both
// succeed without changing anything.
type Patcher interface {
	// AgentId is the agent.Fingerprint ID this patcher handles.
	AgentId() string
	// Supported reports whether this patcher actually does anything. A stub
	// registered as a placeholder for an unimplemented agent returns false.
	Supported() bool
	// Status probes the target and reports whether it is currently patched.
	Status(target Target) (Status, error)
	// Patch installs or refreshes the agent's hooks.
	Patch(target Target) error
	// Unpatch removes everything Patch installed. Patchers that edit shared,
	// mergeable settings must preserve unrelated changes made later.
	Unpatch(target Target) error
}

var patchers = map[string]Patcher{}

// register adds a patcher to the registry. Called from each patcher's init.
func register(patcher Patcher) {
	patchers[patcher.AgentId()] = patcher
}

// SupportedAgents lists the agent IDs that can actually be patched today.
func SupportedAgents() []string {
	var ids []string
	for id, patcher := range patchers {
		if patcher.Supported() {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// StatusOf reports the target's patch status. It never fails: an unknown agent,
// or a probe that errors, is reported as unpatched with the reason in Detail,
// because this is called once per row when listing agents and a broken probe
// must not take the whole page down.
func StatusOf(target Target) Status {
	patcher, ok := patchers[target.AgentId]
	if !ok {
		return Status{Detail: "no patcher registered for this agent"}
	}
	status, err := patcher.Status(target)
	if err != nil {
		return Status{Supported: patcher.Supported(), Detail: err.Error()}
	}
	status.Supported = patcher.Supported()
	return status
}

// Patch instruments the target so it reports its behaviour to aiguard.
func Patch(target Target) error {
	patcher, err := patcherFor(target)
	if err != nil {
		return err
	}
	return patcher.Patch(target)
}

// Unpatch reverses Patch, restoring every file and directory it touched.
func Unpatch(target Target) error {
	patcher, err := patcherFor(target)
	if err != nil {
		return err
	}
	return patcher.Unpatch(target)
}

func patcherFor(target Target) (Patcher, error) {
	if target.AgentId == "" {
		return nil, errors.New("agentId is required")
	}
	if target.Path == "" {
		return nil, errors.New("path is required")
	}
	patcher, ok := patchers[target.AgentId]
	if !ok {
		return nil, fmt.Errorf("unknown agent %q", target.AgentId)
	}
	if !patcher.Supported() {
		return nil, fmt.Errorf("%s: %w", target.AgentId, ErrNotSupported)
	}
	return patcher, nil
}
