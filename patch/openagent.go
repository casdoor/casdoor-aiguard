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

package patch

import "github.com/casdoor/casdoor-aiguard/agentmonitor"

// OpenAgent keeps its state in a SQLite database and exposes no hook directory
// or config file to edit, so it cannot be instrumented in place. What it can be
// asked to do is write an append-only JSONL audit log next to that database,
// which this patcher tails read-only - the same non-invasive strategy codex.go
// uses for Codex rollout files. Patch installs no hook and edits no config; it
// only records a monitoring claim, so Unpatch is just dropping that claim.
type openagentPatcher struct{}

func init() {
	register(openagentPatcher{})
}

func (openagentPatcher) AgentId() string { return "openagent" }

func (openagentPatcher) Supported() bool { return true }

func (p openagentPatcher) Status(target Target) (Status, error) {
	patched, detail := agentmonitor.OpenAgentMonitorStatus(target.Path, target.Owner)
	return Status{Patched: patched, Detail: detail}, nil
}

func (p openagentPatcher) Patch(target Target) error {
	auditDir, err := agentmonitor.ResolveOpenAgentAuditDir(target.Path, target.Owner)
	if err != nil {
		return err
	}
	return agentmonitor.EnableOpenAgentMonitor(target.Path, target.Owner, auditDir)
}

func (p openagentPatcher) Unpatch(target Target) error {
	return agentmonitor.DisableOpenAgentMonitor(target.Path, target.Owner)
}
