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

	"github.com/casdoor/casdoor-aiguard/object"
	"github.com/casdoor/casdoor-aiguard/patch"
)

// llmProviderSwitchRequest is what the Web UI posts from the Agents table's
// "Provider" select: the row's target plus the provider id to switch to, or
// "" to go back to the agent's own default.
type llmProviderSwitchRequest struct {
	AgentId    string `json:"agentId"`
	Path       string `json:"path"`
	Owner      string `json:"owner"`
	ProviderId string `json:"providerId"`
}

// llmProviderSwitchResponse echoes the new state plus a human hint, since
// Claude Code CLI only reads settings.json at process start.
type llmProviderSwitchResponse struct {
	ProviderId string `json:"providerId"`
	Detail     string `json:"detail"`
}

// SetLLMProvider
// @Title SetLLMProvider
// @Description point one agent installation at a saved LLM provider profile, or back to its own default when providerId is empty
// @Param body body llmProviderSwitchRequest true "the installation and provider to switch to"
// @router /agents/llm-provider [post]
func (c *ApiController) SetLLMProvider() {
	var req llmProviderSwitchRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError(err.Error())
		return
	}
	target := patch.Target{AgentId: req.AgentId, Path: req.Path, Owner: req.Owner}

	if !patch.LLMSwitchSupported(target.AgentId) {
		c.ResponseError(fmt.Sprintf("%s does not support switching LLM providers yet", target.AgentId))
		return
	}

	// MutateSettings holds its lock across the whole lookup -> write-the-config
	// -> record-active sequence, so a concurrent switch cannot read the same
	// starting settings and clobber this one's Active entry.
	_, err := object.MutateSettings(func(settings *object.Settings) (*object.Settings, error) {
		if req.ProviderId == "" {
			if err := patch.ClearProvider(target); err != nil {
				return nil, err
			}
		} else {
			provider, found := settings.LLM.ProviderById(req.ProviderId)
			if !found {
				return nil, fmt.Errorf("unknown LLM provider %q", req.ProviderId)
			}
			if err := patch.ApplyProvider(target, patch.LLMProvider{
				BaseUrl:        provider.BaseUrl,
				ApiKey:         provider.ApiKey,
				Model:          provider.Model,
				SmallFastModel: provider.SmallFastModel,
			}); err != nil {
				return nil, err
			}
		}

		settings.LLM.SetActive(target.AgentId, target.Path, target.Owner, req.ProviderId)
		return settings, nil
	})
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	c.ResponseOk(llmProviderSwitchResponse{
		ProviderId: req.ProviderId,
		Detail:     "restart the agent to use this provider - it reads its config only at launch",
	})
}
