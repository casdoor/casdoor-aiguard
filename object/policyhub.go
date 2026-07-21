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
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/casdoor/casdoor-aiguard/conf"
)

// Text is a block of newline-separated text (a Casbin model, policy or request
// list). JSON files may spell it either as one string with "\n" escapes or,
// more readably, as an array of lines. It always marshals back out as a string
// so the Web UI only ever sees one shape.
type Text string

func (t *Text) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*t = Text(s)
		return nil
	}

	var lines []string
	if err := json.Unmarshal(data, &lines); err != nil {
		return fmt.Errorf("a text block must be a string or an array of lines: %w", err)
	}
	*t = Text(strings.Join(lines, "\n"))
	return nil
}

func (t Text) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(t))
}

// PolicySet is one card of the Policy Hub: a ready-made Casbin model plus its
// policy and a few example requests, written for one agent on one operating
// system. Each set lives in its own JSON file under conf.GetPolicyHubDir().
//
// Agent and Os are deliberately single-valued: a policy names the agent it
// guards in every one of its rules, and the hosts it allows (distribution
// mirrors, update channels) are those of one operating system, so a set that
// claimed two of either would be wrong about at least one of them.
type PolicySet struct {
	// Name is the file name without ".json" and identifies the set in URLs.
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Icon        string `json:"icon,omitempty"`
	Author      string `json:"author,omitempty"`
	// Strictness is how much the set restricts an agent: "strict", "moderate"
	// or "permissive".
	Strictness string `json:"strictness,omitempty"`
	// Agent is the agent ID the policy is written for, e.g. "claude-code".
	Agent string `json:"agent,omitempty"`
	// Os is one operating system, a Linux distribution included, e.g. "Windows"
	// or "Ubuntu".
	Os   string   `json:"os,omitempty"`
	Tags []string `json:"tags,omitempty"`

	Model   Text `json:"model"`
	Policy  Text `json:"policy"`
	Request Text `json:"request"`
}

// GetPolicySets reads every JSON file in the policy hub directory. The
// directory is scanned on each call, so dropping in a new file publishes a new
// policy set without a restart. A missing directory simply means an empty hub.
func GetPolicySets() ([]*PolicySet, error) {
	dir := conf.GetPolicyHubDir()

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []*PolicySet{}, nil
	} else if err != nil {
		return nil, err
	}

	policySets := []*PolicySet{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}

		policySet, err := readPolicySet(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		policySets = append(policySets, policySet)
	}

	sort.Slice(policySets, func(i, j int) bool {
		return policySets[i].Name < policySets[j].Name
	})
	return policySets, nil
}

// GetPolicySet reads a single policy set by name, i.e. by its file name without
// the extension. It returns nil when no such set exists.
func GetPolicySet(name string) (*PolicySet, error) {
	if name == "" || strings.ContainsAny(name, `/\.`) {
		return nil, fmt.Errorf("invalid policy set name: %s", name)
	}

	path := filepath.Join(conf.GetPolicyHubDir(), name+".json")
	policySet, err := readPolicySet(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return policySet, err
}

func readPolicySet(path string) (*PolicySet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var policySet PolicySet
	if err := json.Unmarshal(data, &policySet); err != nil {
		return nil, fmt.Errorf("failed to parse policy set %s: %w", filepath.Base(path), err)
	}

	// The file name is the identity, so a set can never disagree with its path.
	policySet.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if policySet.DisplayName == "" {
		policySet.DisplayName = policySet.Name
	}
	return &policySet, nil
}
