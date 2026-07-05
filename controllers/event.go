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
	"strconv"

	"github.com/casdoor/casdoor-aiguard/object"
)

// GetEvents
// @Title GetEvents
// @Description list the most recently intercepted events, newest first
// @Param limit query int false "max number of events to return (default 200)"
// @router /events [get]
func (c *ApiController) GetEvents() {
	limit := 200
	if v := c.Ctx.Input.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			limit = parsed
		}
	}
	c.ResponseOk(object.ListEvents(limit))
}
