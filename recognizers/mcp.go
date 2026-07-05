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

package recognizers

import (
	"encoding/json"
	"net/http"
	"strings"
)

// jsonRpcRequest is the envelope used by the Model Context Protocol, which
// rides on JSON-RPC 2.0. Because it's a structured envelope, the intent
// (which tool, with which arguments) is already explicit - no per-agent
// guessing required.
type jsonRpcRequest struct {
	JsonRpc string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// McpRecognizer recognizes MCP "tools/call" JSON-RPC requests and extracts
// the tool name and arguments as the Intent. It further inspects common
// payment-shaped argument names so a "process_payment" tool call still
// surfaces amount/recipient like the HTTP payment recognizer does.
type McpRecognizer struct{}

func (r *McpRecognizer) Name() string { return "mcp" }

func (r *McpRecognizer) Recognize(host string, req *http.Request, body []byte) (*Intent, bool) {
	if req.Method != http.MethodPost {
		return nil, false
	}
	ct := req.Header.Get("Content-Type")
	if ct != "" && !strings.Contains(ct, "json") {
		return nil, false
	}

	var rpc jsonRpcRequest
	if err := json.Unmarshal(body, &rpc); err != nil {
		return nil, false
	}
	if rpc.JsonRpc != "2.0" || rpc.Method != "tools/call" {
		return nil, false
	}

	var params mcpToolCallParams
	if err := json.Unmarshal(rpc.Params, &params); err != nil || params.Name == "" {
		return nil, false
	}

	intent := &Intent{
		Category: "mcp.tool_call",
		Action:   rpc.Method,
		ToolName: params.Name,
		Raw:      params.Arguments,
	}
	enrichWithPaymentFields(intent, params.Arguments)

	return intent, true
}

// enrichWithPaymentFields looks for common amount/recipient argument names so
// that a payment-shaped MCP tool call (e.g. process_payment) gets the same
// Amount/Recipient/Currency fields a policy rule matches on, regardless of
// whether it arrived over MCP or a plain HTTP API.
func enrichWithPaymentFields(intent *Intent, args map[string]interface{}) {
	if args == nil {
		return
	}
	if amount, ok := numberField(args, "amount", "value", "total"); ok {
		intent.Amount = amount
		intent.Category = "payment"
	}
	if recipient, ok := stringField(args, "recipient", "payee", "to", "destination_account"); ok {
		intent.Recipient = recipient
	}
	if currency, ok := stringField(args, "currency", "currency_code"); ok {
		intent.Currency = currency
	}
}

func numberField(m map[string]interface{}, keys ...string) (float64, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch n := v.(type) {
			case float64:
				return n, true
			case int:
				return float64(n), true
			}
		}
	}
	return 0, false
}

func stringField(m map[string]interface{}, keys ...string) (string, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s, true
			}
		}
	}
	return "", false
}
