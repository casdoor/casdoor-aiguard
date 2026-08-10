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

	"github.com/casdoor/casdoor-aiguard/agentconfig"
)

// GetAgentLlmApi
// @Title GetAgentLlmApi
// @Description read the active LLM API configuration for an agent installation without exposing its API key
// @router /agents/llm-api [get]
func (c *ApiController) GetAgentLlmApi() {
	config, err := agentconfig.Get(agentconfig.Target{
		AgentId: c.Ctx.Input.Query("agentId"),
		Path:    c.Ctx.Input.Query("path"),
		Owner:   c.Ctx.Input.Query("owner"),
	})
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(config)
}

// UpdateAgentLlmApi
// @Title UpdateAgentLlmApi
// @Description switch an agent installation between its official and relay LLM API configuration
// @router /agents/llm-api [post]
func (c *ApiController) UpdateAgentLlmApi() {
	var update agentconfig.Update
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &update); err != nil {
		c.ResponseError(err.Error())
		return
	}

	config, err := agentconfig.Set(update)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(config)
}
