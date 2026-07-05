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

// paymentRequestBody is the shape this example recognizer understands:
// a plain JSON POST body carrying an amount, currency and recipient. It
// stands in for "some payment provider's REST API" - written once here,
// and reused for every agent that happens to call an API shaped like this,
// without any of those agents needing to know aiguard exists.
type paymentRequestBody struct {
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
	Recipient string  `json:"recipient"`
	Payee     string  `json:"payee"`
}

// PaymentRecognizer recognizes plain HTTP payment API calls, e.g.
// POST /v1/payments or POST /v1/charges with a JSON body containing an
// amount and a recipient/payee.
type PaymentRecognizer struct{}

func (r *PaymentRecognizer) Name() string { return "payment-example" }

func (r *PaymentRecognizer) Recognize(host string, req *http.Request, body []byte) (*Intent, bool) {
	if req.Method != http.MethodPost {
		return nil, false
	}
	if !strings.Contains(req.URL.Path, "/payments") && !strings.Contains(req.URL.Path, "/charges") {
		return nil, false
	}

	var payload paymentRequestBody
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false
	}
	if payload.Amount <= 0 {
		return nil, false
	}

	recipient := payload.Recipient
	if recipient == "" {
		recipient = payload.Payee
	}

	raw := map[string]interface{}{
		"amount":    payload.Amount,
		"currency":  payload.Currency,
		"recipient": recipient,
		"path":      req.URL.Path,
	}

	return &Intent{
		Category:  "payment",
		Action:    "process_payment",
		Amount:    payload.Amount,
		Currency:  payload.Currency,
		Recipient: recipient,
		Raw:       raw,
	}, true
}
