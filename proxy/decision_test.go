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

package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/casdoor/casdoor-aiguard/casdoorclient"
	"github.com/casdoor/casdoor-aiguard/object"
	"github.com/casdoor/casdoor-aiguard/recognizers"
)

// fakeCasdoor stands in for Casdoor's OAuth token endpoint and Casbin
// enforce API, so the PEP -> PDP call path can be exercised end-to-end
// without a real Casdoor deployment. allowed controls what /api/enforce
// returns.
func fakeCasdoor(t *testing.T, allowed bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "test-token",
			"expires_in":   3600,
		})
	})
	mux.HandleFunc("/api/enforce", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"data":   allowed,
		})
	})
	return httptest.NewServer(mux)
}

func newTestEngine(t *testing.T, casdoorURL string) *Engine {
	t.Helper()

	object.SetSettings(&object.Settings{
		Casdoor: object.CasdoorSettings{
			Endpoint:     casdoorURL,
			ClientId:     "test-client",
			ClientSecret: "test-secret",
			Organization: "built-in",
			Application:  "app-built-in",
		},
		Intercept: object.InterceptSettings{
			ProxyPort:               9090,
			FailClosedOnPdpError:    true,
			PassthroughUnrecognized: true,
			StepUpDefaultAction:     "deny",
		},
	})
	object.SetPolicy(&object.Policy{
		EnabledRecognizers: []string{"mcp", "payment-example"},
		DefaultAction:      object.ActionPdp,
		Rules: []object.Rule{
			{Id: "high-value-step-up", Category: "payment", MinAmount: 1000, Action: object.ActionStepUp},
			{Id: "payment-to-pdp", Category: "payment", Action: object.ActionPdp},
		},
	})

	return &Engine{
		registry: recognizers.Default(),
		pdp:      casdoorclient.NewClient(),
	}
}

func paymentRequest(amount float64, recipient string) (*http.Request, []byte) {
	body, _ := json.Marshal(map[string]interface{}{
		"amount":    amount,
		"currency":  "USD",
		"recipient": recipient,
	})
	req := httptest.NewRequest(http.MethodPost, "https://payments.example.com/v1/payments", bytes.NewReader(body))
	return req, body
}

func TestDecide_LowValuePayment_AllowedByPdp(t *testing.T) {
	casdoor := fakeCasdoor(t, true)
	defer casdoor.Close()

	engine := newTestEngine(t, casdoor.URL)
	req, body := paymentRequest(50, "acme-corp")

	action, reason := engine.Decide("payments.example.com", req, body, SourceMeta{ProcessName: "test-agent"})
	if action != object.ActionAllow {
		t.Fatalf("expected allow, got %s (%s)", action, reason)
	}
}

func TestDecide_LowValuePayment_DeniedByPdp(t *testing.T) {
	casdoor := fakeCasdoor(t, false)
	defer casdoor.Close()

	engine := newTestEngine(t, casdoor.URL)
	req, body := paymentRequest(50, "acme-corp")

	action, _ := engine.Decide("payments.example.com", req, body, SourceMeta{ProcessName: "test-agent"})
	if action != object.ActionDeny {
		t.Fatalf("expected deny, got %s", action)
	}
}

func TestDecide_HighValuePayment_StepUpStubbedToDeny(t *testing.T) {
	casdoor := fakeCasdoor(t, true) // PDP would allow, but the step-up rule should short-circuit first
	defer casdoor.Close()

	engine := newTestEngine(t, casdoor.URL)
	req, body := paymentRequest(5000, "acme-corp")

	action, reason := engine.Decide("payments.example.com", req, body, SourceMeta{ProcessName: "test-agent"})
	if action != object.ActionDeny {
		t.Fatalf("expected step-up to resolve to deny, got %s (%s)", action, reason)
	}
}

func TestDecide_UnreachablePdp_FailsClosed(t *testing.T) {
	engine := newTestEngine(t, "http://127.0.0.1:1") // nothing listening there
	req, body := paymentRequest(50, "acme-corp")

	action, _ := engine.Decide("payments.example.com", req, body, SourceMeta{ProcessName: "test-agent"})
	if action != object.ActionDeny {
		t.Fatalf("expected fail-closed deny when Casdoor is unreachable, got %s", action)
	}
}

func TestDecide_UnrecognizedTraffic_PassesThrough(t *testing.T) {
	casdoor := fakeCasdoor(t, true)
	defer casdoor.Close()

	engine := newTestEngine(t, casdoor.URL)
	req := httptest.NewRequest(http.MethodGet, "https://example.com/some/random/api", nil)

	action, reason := engine.Decide("example.com", req, nil, SourceMeta{ProcessName: "test-agent"})
	if action != object.ActionAllow {
		t.Fatalf("expected unrecognized traffic to pass through, got %s (%s)", action, reason)
	}
}
