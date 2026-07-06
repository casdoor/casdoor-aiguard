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
	"net/http"

	"github.com/casdoor/casdoor-aiguard/casdoorclient"
	"github.com/casdoor/casdoor-aiguard/object"
	"github.com/casdoor/casdoor-aiguard/recognizers"
)

// SourceMeta is what we could learn about the process that originated an
// intercepted connection. Full SPIFFE-based agent identity is a later stage;
// stage 1 does best-effort PID/process-name resolution via /proc.
type SourceMeta struct {
	Pid         int
	ProcessName string
}

// Decide runs the full PEP pipeline for one intercepted, decrypted HTTP
// request: recognize its intent, evaluate local policy, and optionally call
// Casdoor as the PDP. It returns the final action, a human-readable reason,
// and the (unrecorded) event describing the exchange - already populated with
// the request capture and decision. The caller attaches the upstream response
// and records it, so a single event carries both request and response.
func (e *Engine) Decide(host string, req *http.Request, body []byte, meta SourceMeta) (object.Action, string, *object.Event) {
	policy := object.GetPolicy()

	// build assembles the event for one decision outcome and attaches the
	// captured request, but leaves recording to the caller.
	build := func(recognizerName string, intent *recognizers.Intent, action object.Action, reason string) (object.Action, string, *object.Event) {
		ev := object.NewEvent(host, meta.Pid, meta.ProcessName, recognizerName, intent, action, reason)
		ev.AttachRequest(req, body)
		return action, reason, ev
	}

	intent, recognizerName, recognized := e.registry.Recognize(host, req, body)
	if !recognized {
		action := object.ActionAllow
		if !object.GetSettings().Intercept.PassthroughUnrecognized {
			action = object.ActionDeny
		}
		return build("", nil, action, "unrecognized-traffic")
	}

	if !policy.IsRecognizerEnabled(recognizerName) {
		return build(recognizerName, intent, object.ActionAllow, "recognizer-disabled")
	}

	action, ruleReason := policy.Evaluate(intent, host)

	switch action {
	case object.ActionAllow, object.ActionDeny:
		return build(recognizerName, intent, action, ruleReason)

	case object.ActionStepUp:
		// Stage 1 stubs CIBA: the operator configures whether a pending
		// step-up defaults to deny (safe) or allow while no real human
		// approval channel exists yet.
		stubAction := object.Action(object.GetSettings().Intercept.StepUpDefaultAction)
		if stubAction != object.ActionAllow && stubAction != object.ActionDeny {
			stubAction = object.ActionDeny
		}
		reason := "step-up required (CIBA stubbed): " + ruleReason + " -> " + string(stubAction)
		return build(recognizerName, intent, stubAction, reason)

	default: // ActionPdp
		source := meta.ProcessName
		if source == "" {
			source = "unknown-agent"
		}
		decision, reason := e.pdp.Decide(intent, source)
		pdpAction := object.ActionDeny
		if decision == casdoorclient.DecisionAllow {
			pdpAction = object.ActionAllow
		}
		return build(recognizerName, intent, pdpAction, reason)
	}
}
