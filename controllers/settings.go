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

	if err := object.SaveSettings(&s); err != nil {
		c.ResponseError(err.Error())
		return
	}

	c.ResponseOk(object.GetSettings())
}
