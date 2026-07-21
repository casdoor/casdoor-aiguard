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

// unimplemented is a placeholder Patcher for an agent aiguard can discover but
// cannot instrument yet. Registering one, rather than leaving the agent out of
// the registry, is what lets the Web UI tell "we have not written this yet"
// apart from "we have never heard of this agent".
//
// To implement an agent, replace its embedded unimplemented with real Patch,
// Unpatch and Status methods. There are two worked examples to follow from,
// depending on what the agent offers: openclaw.go installs a hook into an agent
// that has an event system, and claude_desktop.go registers aiguard as an MCP
// server for an agent that has none. Either way, Patch must route every
// filesystem edit through the ChangeSet it is handed, which is what makes
// Unpatch a matter of calling Revert.
type unimplemented struct {
	id string
}

func (u unimplemented) AgentId() string { return u.id }

func (u unimplemented) Supported() bool { return false }

func (u unimplemented) Status(Target) (Status, error) {
	return Status{Detail: "not implemented yet"}, nil
}

func (u unimplemented) Patch(Target) error { return ErrNotSupported }

func (u unimplemented) Unpatch(Target) error { return ErrNotSupported }
