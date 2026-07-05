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

// GetPolicy
// @Title GetPolicy
// @Description get the currently active policy (rules, enabled recognizers, destination allowlist)
// @router /policy [get]
func (c *ApiController) GetPolicy() {
	c.ResponseOk(object.GetPolicy())
}

// UpdatePolicy
// @Title UpdatePolicy
// @Description replace the active policy and persist it to the policy file
// @Param body body object.Policy true "the new policy"
// @router /policy [post]
func (c *ApiController) UpdatePolicy() {
	var p object.Policy
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &p); err != nil {
		c.ResponseError(err.Error())
		return
	}

	if err := object.SavePolicy(&p); err != nil {
		c.ResponseError(err.Error())
		return
	}

	c.ResponseOk(object.GetPolicy())
}
