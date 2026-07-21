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

package mcpserver

import "encoding/json"

// The wire format: MCP is JSON-RPC 2.0 with one message per line.

// JSON-RPC error codes used here.
const (
	invalidParams  = -32602
	methodNotFound = -32601
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	Id      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	Id      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (r request) reply(result any) response {
	return response{Id: r.Id, Result: result}
}

func (r request) fail(code int, message string) response {
	return response{Id: r.Id, Error: &rpcError{Code: code, Message: message}}
}

// paramsMap decodes the request parameters for recording. Parameters that are
// not an object (or absent) simply produce no payload, since the record already
// carries the method name.
func (r request) paramsMap() map[string]any {
	if len(r.Params) == 0 {
		return nil
	}
	var params map[string]any
	if err := json.Unmarshal(r.Params, &params); err != nil {
		return nil
	}
	return params
}

// reportActionTool is the one tool aiguard exposes. It exists so that an agent
// which wants to declare what it is about to do has somewhere to declare it -
// every call lands in the audit log as a record.
const reportActionTool = "aiguard_report_action"

var tools = []map[string]any{{
	"name":        reportActionTool,
	"description": "Report an action you are about to take to the Casdoor AIGuard audit log. Use this before performing a sensitive operation such as a payment, a credential change, or sending data to a third party.",
	"inputSchema": map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Short description of the action, e.g. 'transfer funds' or 'send customer list by email'.",
			},
			"detail": map[string]any{
				"type":        "string",
				"description": "Any relevant detail: amounts, recipients, files, endpoints.",
			},
		},
		"required": []string{"action"},
	},
}}
