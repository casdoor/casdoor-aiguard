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
	"strings"

	"github.com/casdoor/casdoor-aiguard/object"
)

// enforceRequest is what a patched agent's intercept path posts to ask for a
// verdict on one operation. It carries the same fields as a behaviour record -
// the operation is logged either way - plus the two the enforcer needs: the
// resource (the Casbin object, e.g. "127.0.0.1#delete_file") and the intent (the
// Casbin action, e.g. "mcp.tool_call").
type enforceRequest struct {
	object.Record
	Resource string `json:"resource"`
	Intent   string `json:"intent"`
}

// Enforce is the interception counterpart to AddRecord: where AddRecord only
// logs, Enforce also rules. A patched agent posts the operation it is about to
// perform; aiguard evaluates every enabled policy set written for that agent and
// answers allow or deny. The agent blocks the operation when the answer is deny.
//
// The operation is recorded whatever the verdict, with a denied one marked
// triggered, so the Records page shows both what agents do and what aiguard
// stopped. An agent that cannot reach this endpoint fails open by design: the
// intercept path treats a missing answer as allow, so a stopped aiguard never
// breaks an agent.
//
// @Title Enforce
// @Description rule on one agent operation against the enabled policy sets, and record it
// @router /enforce [post]
func (c *ApiController) Enforce() {
	var req enforceRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if req.Agent == "" {
		c.ResponseError("agent is required")
		return
	}

	// The reporter cannot be trusted to describe where it is calling from, so
	// take the peer address from the connection instead of the body.
	req.ClientIp = strings.TrimSpace(c.Ctx.Input.IP())

	decision := object.EnforceForAgent(req.Agent, req.Resource, req.Intent)

	record := req.Record
	if !decision.Allowed {
		record.IsTriggered = true
	}
	object.AddRecord(&record)

	c.ResponseOk(decision)
}
