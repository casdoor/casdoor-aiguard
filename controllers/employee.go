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

	"github.com/casdoor/casdoor-aiguard/auth"
	"github.com/casdoor/casdoor-aiguard/object"
)

// The digital employee page reads and writes the signed-in person's own policy
// set. It is stored on their Casdoor user rather than on this host, so it is
// the one part of aiguard that requires being signed in: there is no such thing
// as an anonymous employee.

// requireCasdoorUser resolves the signed-in operator to their live Casdoor
// user, answering with the reason instead when that is not possible. The user
// is re-read from Casdoor rather than taken from the session, because the
// session's claims are a snapshot from login time and the properties saved here
// must be edited against what Casdoor holds now.
func (c *ApiController) requireCasdoorUser() *auth.User {
	claims := c.GetSessionClaims()
	if claims == nil {
		c.ResponseError("please sign in to manage your digital employee")
		return nil
	}
	if !auth.IsAvailable() {
		c.ResponseError("Casdoor is not available, check the Casdoor Connection settings")
		return nil
	}

	user, err := auth.GetUser(claims.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return nil
	}
	if user == nil {
		c.ResponseError(fmt.Sprintf("the user %q doesn't exist in Casdoor", claims.Name))
		return nil
	}
	return user
}

// employeePolicySetOf reads a user's saved policy set, falling back to the
// starter template for whichever of the three blocks they have never saved.
// Mixing the two is deliberate: someone who saved only a policy still gets a
// model that can run it.
func employeePolicySetOf(user *auth.User) *object.EmployeePolicySet {
	subject := user.Name
	model, policy, request := object.DefaultEmployeePolicySet(subject)

	stored := false
	take := func(key string, fallback object.Text) object.Text {
		if value, ok := user.Properties[key]; ok && value != "" {
			stored = true
			return object.Text(value)
		}
		return fallback
	}

	set := &object.EmployeePolicySet{
		Owner:       user.Owner,
		Name:        user.Name,
		DisplayName: user.DisplayName,
		Subject:     subject,
		Model:       take(object.EmployeeModelProperty, model),
		Policy:      take(object.EmployeePolicyProperty, policy),
		Request:     take(object.EmployeeRequestProperty, request),
	}
	set.Stored = stored
	if set.DisplayName == "" {
		set.DisplayName = user.Name
	}
	return set
}

// GetEmployeePolicySet
// @Title GetEmployeePolicySet
// @Description get the signed-in user's digital employee policy set - the Casbin
// @Description model, policy and example requests stored in their Casdoor user's
// @Description properties, or a starter template when they have never saved one
// @router /employee-policy-set [get]
func (c *ApiController) GetEmployeePolicySet() {
	user := c.requireCasdoorUser()
	if user == nil {
		return
	}

	c.ResponseOk(employeePolicySetOf(user))
}

// UpdateEmployeePolicySet
// @Title UpdateEmployeePolicySet
// @Description save the signed-in user's digital employee policy set back to
// @Description their Casdoor user's properties
// @router /employee-policy-set [post]
func (c *ApiController) UpdateEmployeePolicySet() {
	var body struct {
		Model   string `json:"model"`
		Policy  string `json:"policy"`
		Request string `json:"request"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &body); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if body.Model == "" {
		c.ResponseError("the Casbin model can't be empty")
		return
	}

	user := c.requireCasdoorUser()
	if user == nil {
		return
	}

	if user.Properties == nil {
		user.Properties = map[string]string{}
	}
	user.Properties[object.EmployeeModelProperty] = body.Model
	user.Properties[object.EmployeePolicyProperty] = body.Policy
	user.Properties[object.EmployeeRequestProperty] = body.Request

	if err := auth.UpdateUserProperties(user); err != nil {
		c.ResponseError(err.Error())
		return
	}

	c.ResponseOk(employeePolicySetOf(user))
}
