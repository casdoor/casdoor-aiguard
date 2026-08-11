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

	"github.com/casdoor/casdoor-aiguard/auth"
	"github.com/casdoor/casdoor-aiguard/object"
)

// GetSettings
// @Title GetSettings
// @Description get the live Casdoor connection and interception settings
// @router /settings [get]
func (c *ApiController) GetSettings() {
	c.ResponseOk(object.GetSettings())
}

// UpdateSettings
// @Title UpdateSettings
// @Description replace the live settings and persist them to settings.yaml.
// @Description Note: interception settings (e.g. proxyPort) only take effect
// @Description after aiguard is restarted.
// @Param body body object.Settings true "the new settings"
// @router /settings [post]
func (c *ApiController) UpdateSettings() {
	var s object.Settings
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &s); err != nil {
		c.ResponseError(err.Error())
		return
	}

	// MutateSettings serializes this validate -> merge -> persist sequence
	// against every other settings update, including a concurrent
	// /api/agents/llm-provider switch.
	updated, err := object.MutateSettings(func(current *object.Settings) (*object.Settings, error) {
		if err := current.LLM.ValidateProviderRemoval(s.LLM); err != nil {
			return nil, err
		}

		// The client never receives an existing provider's API key, so one it
		// posts back without a key is not asking to clear it - it just
		// round-tripped what it was given.
		s.LLM.PreserveApiKeys(current.LLM)
		return &s, nil
	})
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	// Operator login points at the same Casdoor instance, so it has to follow
	// the edit rather than wait for a restart.
	auth.InitConfig()

	c.ResponseOk(updated)
}
