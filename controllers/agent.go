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

package controllers

import (
	"encoding/json"

	"github.com/casdoor/casdoor-aiguard/agent"
	"github.com/casdoor/casdoor-aiguard/agentconfig"
	"github.com/casdoor/casdoor-aiguard/patch"
)

// discoveredAgent is one row of the Web UI's agents table: what the scan found,
// whether that installation is patched, and whether its LLM API is configurable.
type discoveredAgent struct {
	agent.Installation
	patch.Status
	ApiConfigurable bool `json:"apiConfigurable"`
}

// GetAgents
// @Title GetAgents
// @Description scan known installation locations and list AI agents installed on the host, each with its patch status
// @router /agents [get]
func (c *ApiController) GetAgents() {
	installations := agent.Scan()

	agents := make([]*discoveredAgent, 0, len(installations))
	for _, installation := range installations {
		agents = append(agents, &discoveredAgent{
			Installation:    installation,
			Status:          patch.StatusOf(targetOf(installation)),
			ApiConfigurable: agentconfig.Supports(installation.AgentId),
		})
	}
	c.ResponseOk(agents)
}

// PatchAgent
// @Title PatchAgent
// @Description install aiguard's hooks into one agent installation, so it reports its behaviour as records
// @router /agents/patch [post]
func (c *ApiController) PatchAgent() {
	target, ok := c.readTarget()
	if !ok {
		return
	}
	if err := patch.Patch(target); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(patch.StatusOf(target))
}

// UnpatchAgent
// @Title UnpatchAgent
// @Description remove aiguard's hooks from one agent installation, restoring every file the patch changed
// @router /agents/unpatch [post]
func (c *ApiController) UnpatchAgent() {
	target, ok := c.readTarget()
	if !ok {
		return
	}
	if err := patch.Unpatch(target); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(patch.StatusOf(target))
}

// readTarget decodes the installation to act on from the request body, replying
// with the error itself when the body does not parse.
func (c *ApiController) readTarget() (patch.Target, bool) {
	var target patch.Target
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &target); err != nil {
		c.ResponseError(err.Error())
		return target, false
	}
	return target, true
}

func targetOf(installation agent.Installation) patch.Target {
	return patch.Target{
		AgentId: installation.AgentId,
		Path:    installation.Path,
		Owner:   installation.Owner,
	}
}
