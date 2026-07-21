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
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/casdoor/casdoor-aiguard/conf"
)

// policySetStateFileName is the on-disk record of which Policy Hub sets are
// enabled for live interception. It lives beside settings.yaml so a toggle made
// from the Web UI survives a restart, the same way settings do.
const policySetStateFileName = "policyset_state.json"

// policySetState is the persisted enablement of Policy Hub sets. Only the set
// name is stored: everything else about a set is read fresh from its JSON file
// on each request, so recording which files exist here would only go stale.
type policySetState struct {
	Enabled []string `json:"enabled"`
}

var (
	enabledPolicySets = map[string]bool{}
	policySetStateMu  sync.RWMutex
)

func policySetStatePath() string {
	return filepath.Join(filepath.Dir(conf.GetPolicyFile()), policySetStateFileName)
}

// InitPolicySetState loads the enabled-set list into memory. A missing file
// simply means nothing is enabled yet. Call once at startup.
func InitPolicySetState() error {
	data, err := os.ReadFile(policySetStatePath())
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	var state policySetState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	policySetStateMu.Lock()
	defer policySetStateMu.Unlock()
	enabledPolicySets = map[string]bool{}
	for _, name := range state.Enabled {
		enabledPolicySets[name] = true
	}
	return nil
}

// IsPolicySetEnabled reports whether the named set is enabled for interception.
func IsPolicySetEnabled(name string) bool {
	policySetStateMu.RLock()
	defer policySetStateMu.RUnlock()
	return enabledPolicySets[name]
}

// EnabledPolicySetNames returns every enabled set name, sorted, for callers that
// need to iterate them (the enforcer does). The result is never nil.
func EnabledPolicySetNames() []string {
	policySetStateMu.RLock()
	names := make([]string, 0, len(enabledPolicySets))
	for name := range enabledPolicySets {
		names = append(names, name)
	}
	policySetStateMu.RUnlock()

	sort.Strings(names)
	return names
}

// SetPolicySetEnabled turns interception on or off for one set and persists the
// change. The gating check (agent patched, OS matches) is the controller's job;
// this only records the operator's decision.
func SetPolicySetEnabled(name string, enabled bool) error {
	// Drop any cached enforcer so a set edited while disabled recompiles on the
	// next enforcement rather than running stale.
	invalidateEnforcer(name)

	policySetStateMu.Lock()
	if enabled {
		enabledPolicySets[name] = true
	} else {
		delete(enabledPolicySets, name)
	}
	names := make([]string, 0, len(enabledPolicySets))
	for n := range enabledPolicySets {
		names = append(names, n)
	}
	policySetStateMu.Unlock()

	sort.Strings(names)
	data, err := json.MarshalIndent(policySetState{Enabled: names}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(policySetStatePath(), append(data, '\n'), 0o644)
}
