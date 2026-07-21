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

// Package controllers holds aiguard's own management API - the Web UI's
// event stream, policy editor and settings pages talk to these endpoints.
// It never talks to the intercepted agents; that's proxy's job.
package controllers

import (
	"encoding/gob"

	"github.com/beego/beego/v2/server/web"
	"github.com/casdoor/casdoor-aiguard/auth"
)

func init() {
	// Claims travel through beego's session store, which encodes with gob.
	gob.Register(auth.Claims{})
}

// ApiController is the base controller for handlers under /api.
type ApiController struct {
	web.Controller
}

// GetSessionClaims returns the signed-in operator's claims, or nil when the
// session is anonymous - the normal state, since login is optional.
func (c *ApiController) GetSessionClaims() *auth.Claims {
	s := c.GetSession("user")
	if s == nil {
		return nil
	}

	claims, ok := s.(auth.Claims)
	if !ok {
		return nil
	}
	return &claims
}

// SetSessionClaims signs the operator in, or signs them out when claims is nil.
func (c *ApiController) SetSessionClaims(claims *auth.Claims) {
	if claims == nil {
		c.DelSession("user")
		return
	}

	c.SetSession("user", *claims)
}

// GetSessionUser returns the signed-in operator, or nil when anonymous.
func (c *ApiController) GetSessionUser() *auth.User {
	claims := c.GetSessionClaims()
	if claims == nil {
		return nil
	}
	return &claims.User
}

// Response is the envelope every /api endpoint replies with, mirroring Casdoor's.
type Response struct {
	Status string      `json:"status"`
	Msg    string      `json:"msg"`
	Data   interface{} `json:"data"`
}

func (c *ApiController) ResponseOk(data ...interface{}) {
	resp := &Response{Status: "ok"}
	if len(data) > 0 {
		resp.Data = data[0]
	}
	c.Data["json"] = resp
	c.ServeJSON()
}

func (c *ApiController) ResponseError(errorMsg string) {
	c.Data["json"] = &Response{Status: "error", Msg: errorMsg}
	c.ServeJSON()
}
