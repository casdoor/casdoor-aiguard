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
	"runtime"
	"sort"
	"strings"

	"github.com/casdoor/casdoor-aiguard/conf"
)

// osFamilies groups operating systems the way the Web UI's filter does (see
// web/src/pages/policySetUtil.js): a set names one OS, but a distribution
// belongs to the Linux family, so a Linux host may enforce any Linux set.
var osFamilies = map[string]string{
	"Windows": "Windows",
	"macOS":   "macOS",
	"Ubuntu":  "Linux", "Debian": "Linux", "CentOS": "Linux", "RHEL": "Linux",
	"Fedora": "Linux", "Arch Linux": "Linux", "openSUSE": "Linux", "Alpine": "Linux",
}

// osFamilyOf maps one OS to its family, falling back to the OS itself for a
// distribution the table above does not list yet.
func osFamilyOf(os string) string {
	if family, ok := osFamilies[os]; ok {
		return family
	}
	return os
}

// HostOsFamily is the family of the operating system aiguard is running on, in
// the same vocabulary a set's "os" field uses. It is what a set's OS is checked
// against before the set may be enabled.
func HostOsFamily() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	default:
		return "Linux"
	}
}

// MatchesHostOs reports whether a set targeting the given OS may run on this
// host, i.e. whether the two share an OS family.
func MatchesHostOs(setOs string) bool {
	return osFamilyOf(setOs) == HostOsFamily()
}

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

	// The fields below are computed per request, not read from the set's file:
	// they describe whether this set is live and whether it may be turned on for
	// the agents and OS present on this host. A file never sets them, so they
	// marshal as their zero values until the controller fills them in.

	// Enabled reports whether this set is currently enforcing on patched agents.
	Enabled bool `json:"enabled"`
	// CanEnable reports whether the operator may turn this set on right now; an
	// already-enabled set can always be turned off, regardless of this.
	CanEnable bool `json:"canEnable"`
	// EnableReason explains, when CanEnable is false, why the set cannot be
	// enabled - shown as the toggle's tooltip.
	EnableReason string `json:"enableReason,omitempty"`
}

// interceptCapableAgents is the set of agents aiguard can inject a policy
// decision into today, keyed by the display name a policy set stores in "agent".
// OpenClaw is guarded by the hook in patch/openclaw_hook.js. Every other agent
// is instrumented or monitored for reporting only, so its sets cannot be
// enabled until an intercept path exists.
var interceptCapableAgents = map[string]bool{
	"OpenClaw": true,
}

// IsInterceptCapableAgent reports whether a set written for this agent (by its
// "agent" display name) can be enforced on the host at all.
func IsInterceptCapableAgent(agent string) bool {
	return interceptCapableAgents[agent]
}

// InterceptCapableAgentNames lists, sorted, the agents aiguard can enforce
// policies on today. Used to tell the operator which agents a set could target
// when the one it names isn't supported yet.
func InterceptCapableAgentNames() []string {
	names := make([]string, 0, len(interceptCapableAgents))
	for name := range interceptCapableAgents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
