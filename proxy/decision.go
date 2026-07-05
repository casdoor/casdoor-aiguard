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
// request: recognize its intent, evaluate local policy, optionally call
// Casdoor as the PDP, and record the resulting event. It returns the final
// action to take and a human-readable reason for it.
func (e *Engine) Decide(host string, req *http.Request, body []byte, meta SourceMeta) (object.Action, string) {
	policy := object.GetPolicy()

	intent, recognizerName, recognized := e.registry.Recognize(host, req, body)
	if !recognized {
		action := object.ActionAllow
		if !object.GetSettings().Intercept.PassthroughUnrecognized {
			action = object.ActionDeny
		}
		object.RecordEvent(object.NewEvent(host, meta.Pid, meta.ProcessName, "", nil, action, "unrecognized-traffic"))
		return action, "unrecognized-traffic"
	}

	if !policy.IsRecognizerEnabled(recognizerName) {
		object.RecordEvent(object.NewEvent(host, meta.Pid, meta.ProcessName, recognizerName, intent, object.ActionAllow, "recognizer-disabled"))
		return object.ActionAllow, "recognizer-disabled"
	}

	action, ruleReason := policy.Evaluate(intent, host)

	switch action {
	case object.ActionAllow, object.ActionDeny:
		object.RecordEvent(object.NewEvent(host, meta.Pid, meta.ProcessName, recognizerName, intent, action, ruleReason))
		return action, ruleReason

	case object.ActionStepUp:
		// Stage 1 stubs CIBA: the operator configures whether a pending
		// step-up defaults to deny (safe) or allow while no real human
		// approval channel exists yet.
		stubAction := object.Action(object.GetSettings().Intercept.StepUpDefaultAction)
		if stubAction != object.ActionAllow && stubAction != object.ActionDeny {
			stubAction = object.ActionDeny
		}
		reason := "step-up required (CIBA stubbed): " + ruleReason + " -> " + string(stubAction)
		object.RecordEvent(object.NewEvent(host, meta.Pid, meta.ProcessName, recognizerName, intent, stubAction, reason))
		return stubAction, reason

	default: // ActionPdp
		return e.decideViaPdp(host, intent, recognizerName, meta)
	}
}

func (e *Engine) decideViaPdp(host string, intent *recognizers.Intent, recognizerName string, meta SourceMeta) (object.Action, string) {
	source := meta.ProcessName
	if source == "" {
		source = "unknown-agent"
	}

	decision, reason := e.pdp.Decide(intent, source)

	var action object.Action
	switch decision {
	case casdoorclient.DecisionAllow:
		action = object.ActionAllow
	default:
		action = object.ActionDeny
	}

	object.RecordEvent(object.NewEvent(host, meta.Pid, meta.ProcessName, recognizerName, intent, action, reason))
	return action, reason
}
