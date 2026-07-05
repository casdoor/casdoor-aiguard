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

import "net/http"

// Registry holds the set of enabled recognizers and tries them in order.
type Registry struct {
	recognizers []Recognizer
}

func NewRegistry(rs ...Recognizer) *Registry {
	return &Registry{recognizers: rs}
}

// Default returns the registry enabled by default in stage 1: MCP JSON-RPC
// tool calls, and the example payment recognizer.
func Default() *Registry {
	return NewRegistry(
		&McpRecognizer{},
		&PaymentRecognizer{},
	)
}

// Recognize runs every enabled recognizer against the request until one matches.
func (r *Registry) Recognize(host string, req *http.Request, body []byte) (*Intent, string, bool) {
	for _, rec := range r.recognizers {
		if intent, ok := rec.Recognize(host, req, body); ok {
			intent.Destination = host
			return intent, rec.Name(), true
		}
	}
	return nil, "", false
}
