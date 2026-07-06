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

package object

import (
	"os"
	"sync"

	"github.com/casdoor/casdoor-aiguard/conf"
	"github.com/casdoor/casdoor-aiguard/recognizers"
	"gopkg.in/yaml.v3"
)

// Action is what a matched Rule (or the final PDP call) resolves to.
type Action string

const (
	// ActionAllow lets the request through immediately, no Casdoor call needed.
	ActionAllow Action = "allow"
	// ActionDeny blocks the request immediately, no Casdoor call needed.
	ActionDeny Action = "deny"
	// ActionStepUp suspends the request pending human approval (CIBA-stubbed in stage 1).
	ActionStepUp Action = "step-up"
	// ActionPdp defers the decision to Casdoor's enforce API.
	ActionPdp Action = "pdp"
)

// Rule is one line of the local policy file, e.g. "payments over 1000 need step-up".
// Rules are evaluated top to bottom; the first one whose conditions all match wins.
type Rule struct {
	Id           string   `yaml:"id" json:"id"`
	Category     string   `yaml:"category" json:"category"` // matches Intent.Category, "*" for any
	ToolName     string   `yaml:"toolName,omitempty" json:"toolName,omitempty"`
	MinAmount    float64  `yaml:"minAmount,omitempty" json:"minAmount,omitempty"`
	Destinations []string `yaml:"destinations,omitempty" json:"destinations,omitempty"`
	Action       Action   `yaml:"action" json:"action"`
}

// Policy is the full set of rules plus which recognizers are enabled and
// which destinations are always allowed straight through.
type Policy struct {
	EnabledRecognizers   []string `yaml:"enabledRecognizers" json:"enabledRecognizers"`
	DestinationAllowlist []string `yaml:"destinationAllowlist,omitempty" json:"destinationAllowlist,omitempty"`
	Rules                []Rule   `yaml:"rules" json:"rules"`
	// DefaultAction applies to a recognized-but-unmatched intent. Defaults to "pdp".
	DefaultAction Action `yaml:"defaultAction" json:"defaultAction"`
}

var (
	currentPolicy *Policy
	policyMutex   sync.RWMutex
)

func defaultPolicy() *Policy {
	return &Policy{
		EnabledRecognizers: []string{"mcp", "llm", "payment-example"},
		DefaultAction:      ActionPdp,
		Rules: []Rule{
			{Id: "high-value-payment-step-up", Category: "payment", MinAmount: 1000, Action: ActionStepUp},
			{Id: "payment-needs-pdp", Category: "payment", Action: ActionPdp},
			// LLM chat/completion traffic is allowed through and fully logged
			// (model, prompt, request and response bodies) rather than sent to
			// the PDP on every message.
			{Id: "llm-allow-and-log", Category: "llm.chat", Action: ActionAllow},
		},
	}
}

// LoadPolicy reads the policy file configured by conf.GetPolicyFile(). If the
// file doesn't exist yet, a default policy is written out and used.
func LoadPolicy() (*Policy, error) {
	path := conf.GetPolicyFile()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		p := defaultPolicy()
		if err := SavePolicy(p); err != nil {
			return nil, err
		}
		return p, nil
	} else if err != nil {
		return nil, err
	}

	var p Policy
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// SavePolicy writes the policy to the configured policy file and updates the
// in-memory copy used by the proxy engine.
func SavePolicy(p *Policy) error {
	data, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	if err := os.WriteFile(conf.GetPolicyFile(), data, 0o644); err != nil {
		return err
	}
	SetPolicy(p)
	return nil
}

func SetPolicy(p *Policy) {
	policyMutex.Lock()
	defer policyMutex.Unlock()
	currentPolicy = p
}

func GetPolicy() *Policy {
	policyMutex.RLock()
	defer policyMutex.RUnlock()
	if currentPolicy == nil {
		return defaultPolicy()
	}
	return currentPolicy
}

// InitPolicy loads the policy file into memory at startup.
func InitPolicy() error {
	p, err := LoadPolicy()
	if err != nil {
		return err
	}
	SetPolicy(p)
	return nil
}

// Evaluate finds the first rule that matches the given intent and destination
// host. If no rule matches, the policy's DefaultAction is returned.
func (p *Policy) Evaluate(intent *recognizers.Intent, host string) (Action, string) {
	for _, hostAllowed := range p.DestinationAllowlist {
		if hostAllowed == host {
			return ActionAllow, "destination-allowlist"
		}
	}

	for _, r := range p.Rules {
		if !r.matches(intent, host) {
			continue
		}
		return r.Action, r.Id
	}

	action := p.DefaultAction
	if action == "" {
		action = ActionPdp
	}
	return action, "default"
}

func (r *Rule) matches(intent *recognizers.Intent, host string) bool {
	if r.Category != "" && r.Category != "*" && r.Category != intent.Category {
		return false
	}
	if r.ToolName != "" && r.ToolName != intent.ToolName {
		return false
	}
	if r.MinAmount > 0 && intent.Amount < r.MinAmount {
		return false
	}
	if len(r.Destinations) > 0 && !contains(r.Destinations, host) {
		return false
	}
	return true
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

// IsRecognizerEnabled reports whether the named recognizer is turned on by policy.
func (p *Policy) IsRecognizerEnabled(name string) bool {
	if len(p.EnabledRecognizers) == 0 {
		return true
	}
	return contains(p.EnabledRecognizers, name)
}
