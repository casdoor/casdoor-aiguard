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
	"net/http"
	"strings"

	"github.com/casdoor/casdoor-aiguard/object"
	"github.com/casdoor/casdoor-aiguard/otelreceiver"
)

// IngestOtelLogs accepts the standard OTLP/HTTP JSON logs envelope emitted by
// Claude Code. A successful OTLP response is the protocol's empty JSON object,
// not the management API's usual response wrapper.
func (c *ApiController) IngestOtelLogs() {
	c.Ctx.Output.Header("Content-Type", "application/json")
	if len(c.Ctx.Input.RequestBody) > otelreceiver.MaxRequestBytes {
		c.Ctx.Output.SetStatus(http.StatusRequestEntityTooLarge)
		_ = c.Ctx.Output.Body([]byte(`{"error":"OTLP logs request exceeds 8 MiB"}`))
		return
	}

	records, err := otelreceiver.Parse(c.Ctx.Input.RequestBody)
	if err != nil {
		c.Ctx.Output.SetStatus(http.StatusBadRequest)
		_ = c.Ctx.Output.Body([]byte(`{"error":"invalid OTLP logs JSON"}`))
		return
	}
	clientIp := strings.TrimSpace(c.Ctx.Input.IP())
	for _, record := range records {
		record.ClientIp = clientIp
		object.AddRecord(record)
	}
	_ = c.Ctx.Output.Body([]byte("{}"))
}
