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

// codexRolloutPatcher is deliberately non-invasive. Patch is an AIGuard-side
// monitoring claim; it never edits Codex config.toml, installs a hook or asks
// the user to trust an external command.
type codexRolloutPatcher struct {
	id string
}

func init() {
	register(codexRolloutPatcher{id: "codex"})
	register(codexRolloutPatcher{id: "codex-cli"})
}

func (p codexRolloutPatcher) AgentId() string { return p.id }
func (p codexRolloutPatcher) Supported() bool { return true }

func (p codexRolloutPatcher) Status(target Target) (Status, error) {
	patched, detail := agentmonitor.CodexMonitorStatus(target.AgentId, target.Path, target.Owner)
	return Status{Patched: patched, Detail: detail}, nil
}

func (p codexRolloutPatcher) Patch(target Target) error {
	codexHome, err := agentmonitor.ResolveCodexHome(target.Path, target.Owner)
	if err != nil {
		return err
	}
	return agentmonitor.EnableCodexMonitor(target.AgentId, target.Path, target.Owner, codexHome)
}

func (p codexRolloutPatcher) Unpatch(target Target) error {
	return agentmonitor.DisableCodexMonitor(target.AgentId, target.Path, target.Owner)
}
