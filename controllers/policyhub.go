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
	"fmt"

	"github.com/casdoor/casdoor-aiguard/agent"
	"github.com/casdoor/casdoor-aiguard/object"
	"github.com/casdoor/casdoor-aiguard/patch"
)

// agentPresence is which agents this host has installed, and which of those are
// patched - the two facts that decide whether a set may be enabled. Keyed by the
// display name a set stores in its "agent" field, so a set matches an
// installation without re-deriving anything.
type agentPresence struct {
	installed map[string]bool
	patched   map[string]bool
}

// scanAgentPresence probes the host once, reusing exactly what the agents table
// shows: the scan for installations and patch.StatusOf for whether each is
// patched right now.
func scanAgentPresence() agentPresence {
	presence := agentPresence{installed: map[string]bool{}, patched: map[string]bool{}}
	for _, installation := range agent.Scan() {
		presence.installed[installation.Name] = true
		if patch.StatusOf(targetOf(installation)).Patched {
			presence.patched[installation.Name] = true
		}
	}
	return presence
}

// gatePolicySet decides whether a set may be turned on right now, and when it may
// not, the single reason to show as the toggle's tooltip. The checks run
// most-fundamental first, so the reason names the blocker actually worth acting
// on: an agent aiguard cannot intercept at all, then a set for a different OS
// (nothing on this host fixes that), then a missing agent, then an unpatched one
// (the one step the operator can take right here).
func gatePolicySet(set *object.PolicySet, presence agentPresence) (bool, string) {
	if set.Agent == "" {
		return false, "This policy set names no agent to enforce on"
	}
	if !object.IsInterceptCapableAgent(set.Agent) {
		return false, fmt.Sprintf("Interception isn't supported for %s yet", set.Agent)
	}
	if !object.MatchesHostOs(set.Os) {
		return false, fmt.Sprintf("This set targets %s; this host runs %s", set.Os, object.HostOsFamily())
	}
	if !presence.installed[set.Agent] {
		return false, fmt.Sprintf("%s isn't installed - install and patch it on the Agents page", set.Agent)
	}
	if !presence.patched[set.Agent] {
		return false, fmt.Sprintf("%s is installed but not patched - patch it on the Agents page first", set.Agent)
	}
	return true, ""
}

// GetPolicySets
// @Title GetPolicySets
// @Description list every policy set published in the Policy Hub, read fresh from the policy hub directory
// @router /policy-sets [get]
func (c *ApiController) GetPolicySets() {
	policySets, err := object.GetPolicySets()
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	presence := scanAgentPresence()
	for _, set := range policySets {
		set.Enabled = object.IsPolicySetEnabled(set.Name)
		set.CanEnable, set.EnableReason = gatePolicySet(set, presence)
	}
	c.ResponseOk(policySets)
}

// TogglePolicySet
// @Title TogglePolicySet
// @Description enable or disable one policy set for live interception on patched agents
// @router /policy-set/enable [post]
func (c *ApiController) TogglePolicySet() {
	var body struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &body); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if body.Name == "" {
		c.ResponseError("name is required")
		return
	}

	set, err := object.GetPolicySet(body.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if set == nil {
		c.ResponseError(fmt.Sprintf("the policy set: %s doesn't exist", body.Name))
		return
	}

	// Re-check the gate on the server: the button could have been enabled in a
	// tab opened before the agent was unpatched or the set edited.
	if body.Enabled {
		if canEnable, reason := gatePolicySet(set, scanAgentPresence()); !canEnable {
			c.ResponseError(reason)
			return
		}
	}
	if err := object.SetPolicySetEnabled(body.Name, body.Enabled); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}

// GetPolicySet
// @Title GetPolicySet
// @Description get one policy set of the Policy Hub, with its Casbin model, policy and example requests
// @Param name query string true "the policy set name"
// @router /policy-set [get]
func (c *ApiController) GetPolicySet() {
	name := c.Ctx.Input.Query("name")

	policySet, err := object.GetPolicySet(name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if policySet == nil {
		c.ResponseError(fmt.Sprintf("the policy set: %s doesn't exist", name))
		return
	}
	c.ResponseOk(policySet)
}
