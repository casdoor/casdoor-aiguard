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
	"fmt"

	"github.com/casdoor/casdoor-aiguard/object"
)

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
	c.ResponseOk(policySets)
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
