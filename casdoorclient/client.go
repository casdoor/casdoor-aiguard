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

// Package casdoorclient implements aiguard's role as a Policy Enforcement
// Point talking to Casdoor, which acts as the Policy Decision Point and
// OAuth authorization server. aiguard authenticates to Casdoor with its own
// client-credentials grant, then asks Casdoor to decide allow/deny (and, in
// later stages, step-up) for every recognized sensitive intent.
package casdoorclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/beego/beego/v2/core/logs"
	"github.com/casdoor/casdoor-aiguard/conf"
	"github.com/casdoor/casdoor-aiguard/object"
	"github.com/casdoor/casdoor-aiguard/recognizers"
)

// Decision is the PDP's verdict for a recognized intent.
type Decision string

const (
	DecisionAllow  Decision = "allow"
	DecisionDeny   Decision = "deny"
	DecisionStepUp Decision = "step-up"
	// DecisionError means Casdoor could not be reached or returned an error;
	// the caller applies its own fail-open/fail-closed policy in this case.
	DecisionError Decision = "error"
)

// Client talks to Casdoor's OAuth token endpoint and enforce API.
type Client struct {
	httpClient *http.Client

	tokenMutex  sync.Mutex
	accessToken string
	expiresAt   time.Time
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{Timeout: 5 * time.Second}}
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	Error       string `json:"error"`
}

// getAccessToken performs (and caches) the client_credentials grant against
// Casdoor's OAuth token endpoint.
func (c *Client) getAccessToken() (string, error) {
	c.tokenMutex.Lock()
	defer c.tokenMutex.Unlock()

	if c.accessToken != "" && time.Now().Before(c.expiresAt) {
		return c.accessToken, nil
	}

	casdoorCfg := object.GetSettings().Casdoor
	endpoint := casdoorCfg.Endpoint
	form := map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     casdoorCfg.ClientId,
		"client_secret": casdoorCfg.ClientSecret,
	}
	body := make([]byte, 0)
	first := true
	for k, v := range form {
		if !first {
			body = append(body, '&')
		}
		first = false
		body = append(body, []byte(fmt.Sprintf("%s=%s", k, v))...)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint+"/api/login/oauth/access_token", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", err
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("casdoor token request failed: %s", tr.Error)
	}

	c.accessToken = tr.AccessToken
	expiresIn := tr.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	c.expiresAt = time.Now().Add(time.Duration(expiresIn-30) * time.Second)

	return c.accessToken, nil
}

// Decide asks Casdoor whether the given intent, from the given source
// process, bound for the given destination, should be allowed. It maps to
// a Casbin enforce call: sub = source process (agent identity stand-in for
// stage 1), obj = destination + action, act = intent category.
//
// Stage 1 only distinguishes allow/deny from Casdoor; step-up is decided
// locally by policy before Decide is ever called (see object.Policy).
func (c *Client) Decide(intent *recognizers.Intent, sourceProcess string) (Decision, string) {
	casdoorCfg := object.GetSettings().Casdoor
	endpoint := casdoorCfg.Endpoint
	if endpoint == "" {
		return decideOnUnreachable("casdoor endpoint not configured")
	}

	token, err := c.getAccessToken()
	if err != nil {
		logs.Warn("casdoorclient: failed to get access token: %v", err)
		return decideOnUnreachable(err.Error())
	}

	sub := sourceProcess
	if sub == "" {
		sub = "unknown-agent"
	}
	obj := intent.Destination
	if intent.ToolName != "" {
		obj = fmt.Sprintf("%s#%s", obj, intent.ToolName)
	}
	act := intent.Category

	reqBody, err := json.Marshal([]string{sub, obj, act})
	if err != nil {
		return decideOnUnreachable(err.Error())
	}

	url := fmt.Sprintf("%s%s?owner=%s&application=%s", endpoint, conf.GetPdpEnforcePath(), casdoorCfg.Organization, casdoorCfg.Application)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return decideOnUnreachable(err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logs.Warn("casdoorclient: enforce call failed: %v", err)
		return decideOnUnreachable(err.Error())
	}
	defer resp.Body.Close()

	var result struct {
		Status string      `json:"status"`
		Data   interface{} `json:"data"`
		Msg    string      `json:"msg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return decideOnUnreachable(err.Error())
	}
	if result.Status != "ok" {
		return decideOnUnreachable(result.Msg)
	}

	allowed, _ := result.Data.(bool)
	if allowed {
		return DecisionAllow, "casdoor:allow"
	}
	return DecisionDeny, "casdoor:deny"
}

// decideOnUnreachable applies the configured fail-closed/fail-open behavior
// when Casdoor cannot be reached or returns an error.
func decideOnUnreachable(reason string) (Decision, string) {
	if object.GetSettings().Intercept.FailClosedOnPdpError {
		return DecisionDeny, "fail-closed: " + reason
	}
	return DecisionAllow, "fail-open: " + reason
}
