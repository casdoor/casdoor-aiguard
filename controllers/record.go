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
	"strconv"
	"strings"

	"github.com/casdoor/casdoor-aiguard/object"
)

// GetRecords
// @Title GetRecords
// @Description list the behaviour records reported by patched agents, newest first
// @Param agent query string false "only records from this agent id (default: all agents)"
// @Param limit query int false "max number of records to return (default 200)"
// @router /records [get]
func (c *ApiController) GetRecords() {
	limit := 200
	if value := c.Ctx.Input.Query("limit"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			limit = parsed
		}
	}
	c.ResponseOk(object.ListRecords(c.Ctx.Input.Query("agent"), limit))
}

// AddRecord is the ingest endpoint the hooks aiguard installs into agents post
// to. It is the one API a patched agent calls, so it stays deliberately small:
// take what the agent reports, stamp the parts only aiguard can know, store it.
//
// @Title AddRecord
// @Description report one behaviour record from a patched agent
// @router /records [post]
func (c *ApiController) AddRecord() {
	var record object.Record
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &record); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if record.Agent == "" {
		c.ResponseError("agent is required")
		return
	}

	// The reporter cannot be trusted to describe where it is calling from, so
	// take the peer address from the connection instead of the body.
	record.ClientIp = strings.TrimSpace(c.Ctx.Input.IP())

	object.AddRecord(&record)
	c.ResponseOk()
}
