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

package patch

import "fmt"

// LLMProvider is the base URL, API key and optional model overrides to write
// into an agent's own config. A plain payload rather than object.LLMProvider,
// so this package keeps no dependency on object.
type LLMProvider struct {
	BaseUrl        string
	ApiKey         string
	Model          string
	SmallFastModel string
}

// LLMSwitcher points one kind of agent at a chosen LLM provider by editing the
// agent's own config file. Unlike Patcher this is not one-time
// instrumentation but a routine, repeatable operation, so there is no Status
// to probe and no undo journal - the two methods just have to be idempotent.
type LLMSwitcher interface {
	// AgentId is the agent.Fingerprint ID this switcher handles.
	AgentId() string
	// Supported reports whether this switcher actually does anything.
	Supported() bool
	// ApplyProvider points target at provider, replacing whatever provider
	// aiguard last applied.
	ApplyProvider(target Target, provider LLMProvider) error
	// ClearProvider removes aiguard-managed provider fields, restoring the
	// agent's own default.
	ClearProvider(target Target) error
}

var llmSwitchers = map[string]LLMSwitcher{}

// registerLLMSwitcher adds a switcher to the registry, called from each
// switcher's init.
func registerLLMSwitcher(s LLMSwitcher) {
	llmSwitchers[s.AgentId()] = s
}

// LLMSwitchSupported reports whether agentId has a working switcher, so the
// Web UI can gate the "Provider" control the way it gates Patch/Unpatch.
func LLMSwitchSupported(agentId string) bool {
	s, ok := llmSwitchers[agentId]
	return ok && s.Supported()
}

// ApplyProvider points target at provider.
func ApplyProvider(target Target, provider LLMProvider) error {
	s, err := llmSwitcherFor(target)
	if err != nil {
		return err
	}
	return s.ApplyProvider(target, provider)
}

// ClearProvider reverts target to the agent's own default.
func ClearProvider(target Target) error {
	s, err := llmSwitcherFor(target)
	if err != nil {
		return err
	}
	return s.ClearProvider(target)
}

func llmSwitcherFor(target Target) (LLMSwitcher, error) {
	if target.AgentId == "" {
		return nil, fmt.Errorf("agentId is required")
	}
	if target.Path == "" {
		return nil, fmt.Errorf("path is required")
	}
	s, ok := llmSwitchers[target.AgentId]
	if !ok || !s.Supported() {
		return nil, fmt.Errorf("%s: %w", target.AgentId, ErrNotSupported)
	}
	return s, nil
}
