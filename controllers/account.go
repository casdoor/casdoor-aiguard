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
	"github.com/casdoor/casdoor-aiguard/auth"
)

// GetAuthConfig
// @Title GetAuthConfig
// @Description get the Casdoor sign-in configuration (never the client secret),
// @Description so the Web UI can build the OAuth authorize URL itself
// @router /auth-config [get]
func (c *ApiController) GetAuthConfig() {
	c.ResponseOk(auth.GetConfig())
}

// Signin
// @Title Signin
// @Description finish the Casdoor OAuth login by exchanging the code for a token
// @Param code  query string true "the OAuth code returned by Casdoor"
// @Param state query string true "the OAuth state returned by Casdoor"
// @router /signin [post]
func (c *ApiController) Signin() {
	if !auth.IsAvailable() {
		c.ResponseError("Casdoor login is not available, check the Casdoor Connection settings")
		return
	}

	code := c.GetString("code")
	state := c.GetString("state")
	if code == "" || state == "" {
		c.ResponseError("the OAuth code or state is empty")
		return
	}

	token, err := auth.GetOAuthToken(code, state)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	claims, err := auth.ParseJwtToken(token.AccessToken)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	claims.AccessToken = token.AccessToken
	c.SetSessionClaims(claims)

	c.ResponseOk(claims)
}

// Signout
// @Title Signout
// @Description drop the signed-in operator's session
// @router /signout [post]
func (c *ApiController) Signout() {
	c.SetSessionClaims(nil)
	c.ResponseOk()
}

// GetAccount
// @Title GetAccount
// @Description get the signed-in operator, or null when nobody is signed in.
// @Description Login is optional in aiguard, so being anonymous is a normal
// @Description answer rather than an error.
// @router /account [get]
func (c *ApiController) GetAccount() {
	claims := c.GetSessionClaims()
	if claims == nil {
		c.ResponseOk(nil)
		return
	}

	c.ResponseOk(claims)
}
