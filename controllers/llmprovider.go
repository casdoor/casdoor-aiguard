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

	// MutateSettingsWithUndo holds its lock across the whole lookup ->
	// write-the-config -> record-active sequence, so a concurrent switch cannot
	// read the same starting settings and clobber this one's Active entry. The
	// undo puts the agent back on the provider it was already using if
	// settings.yaml cannot be written, rather than leaving a live config file
	// the Agents page has no record of.
	_, err := object.MutateSettingsWithUndo(func(settings *object.Settings) (*object.Settings, func() error, error) {
		previousId := settings.LLM.ActiveProviderId(target.AgentId, target.Path, target.Owner)
		if err := switchToLLMProvider(settings, target, req.ProviderId); err != nil {
			return nil, nil, err
		}
		undo := func() error { return switchToLLMProvider(settings, target, previousId) }

		settings.LLM.SetActive(target.AgentId, target.Path, target.Owner, req.ProviderId)
		return settings, undo, nil
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

// switchToLLMProvider writes providerId's endpoint into target's own config,
// or restores the agent's default when providerId is empty. Both directions go
// through here so the undo path in SetLLMProvider is the same code as the
// forward one.
func switchToLLMProvider(settings *object.Settings, target patch.Target, providerId string) error {
	if providerId == "" {
		return patch.ClearProvider(target)
	}
	provider, found := settings.LLM.ProviderById(providerId)
	if !found {
		return fmt.Errorf("unknown LLM provider %q", providerId)
	}
	return patch.ApplyProvider(target, patch.LLMProvider{
		BaseUrl:        provider.BaseUrl,
		ApiKey:         provider.ApiKey,
		Model:          provider.Model,
		SmallFastModel: provider.SmallFastModel,
	})
}
