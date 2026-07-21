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
	"regexp"
	"strings"
	"testing"
)

// policyRule matches "p, <sub>, ..." so the test can read back the subject each
// rule is written for.
var policyRule = regexp.MustCompile(`^p\s*,\s*([^,]+)\s*,`)

// TestBuiltinPolicySets checks the sets shipped under data/policyhub: each one
// must be complete, must name exactly one agent and one operating system, and
// every rule in it must be written for that same agent - a set whose rules
// named two agents would be advertising a guarantee it cannot make for either.
func TestBuiltinPolicySets(t *testing.T) {
	dir := filepath.Join("..", "data", "policyhub")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("no built-in policy sets to check: %v", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		count++

		policySet, err := readPolicySet(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Errorf("%s: %v", entry.Name(), err)
			continue
		}

		for field, value := range map[string]string{
			"description": policySet.Description,
			"agent":       policySet.Agent,
			"os":          policySet.Os,
			"model":       string(policySet.Model),
			"policy":      string(policySet.Policy),
			"request":     string(policySet.Request),
		} {
			if strings.TrimSpace(value) == "" {
				t.Errorf("%s: %s is empty", entry.Name(), field)
			}
		}

		subjects := map[string]bool{}
		for _, line := range strings.Split(string(policySet.Policy), "\n") {
			if match := policyRule.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
				subjects[strings.TrimSpace(match[1])] = true
			}
		}
		if len(subjects) > 1 {
			t.Errorf("%s: policy is written for %d subjects, want 1: %v", entry.Name(), len(subjects), subjects)
		}
	}

	if count == 0 {
		t.Skip("no built-in policy sets to check")
	}
}
