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

// Package recognizers turns raw decrypted HTTP request/response bytes into a
// high-level Intent. Each recognizer is written once against the shape of a
// destination API (or protocol, like MCP) and is reused by every agent that
// happens to call that API - agents are never modified or made aware of this.
package recognizers

import "net/http"

// Intent is the high-level semantic meaning extracted from an intercepted
// request, e.g. "this is a payment of $500 to acme-corp".
type Intent struct {
	// Category groups intents for policy matching, e.g. "payment", "mcp.tool_call".
	Category string `json:"category"`
	// Action is the specific operation, e.g. "process_payment", "tools/call".
	Action string `json:"action"`
	// ToolName is set when the intent originates from an MCP tools/call.
	ToolName string `json:"toolName,omitempty"`
	// Amount and Currency are populated by recognizers that identify a monetary intent.
	Amount   float64 `json:"amount,omitempty"`
	Currency string  `json:"currency,omitempty"`
	// Recipient is the counterparty of a monetary or transactional intent.
	Recipient string `json:"recipient,omitempty"`
	// Destination is the upstream host the request was bound for.
	Destination string `json:"destination,omitempty"`
	// Raw holds the recognizer-specific structured fields for audit/debugging.
	Raw map[string]interface{} `json:"raw,omitempty"`
}

// Recognizer inspects a decrypted request (and its destination host) and,
// if it recognizes the API shape, returns the Intent it extracted.
type Recognizer interface {
	// Name identifies the recognizer, e.g. "mcp", "payment-example".
	Name() string
	// Recognize returns (intent, true) if this recognizer understood the request,
	// or (nil, false) if the request shape didn't match.
	Recognize(host string, req *http.Request, body []byte) (*Intent, bool)
}
