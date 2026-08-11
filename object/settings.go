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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/casdoor/casdoor-aiguard/conf"
	"github.com/casdoor/casdoor-aiguard/util"
	"gopkg.in/yaml.v3"
)

// CasdoorSettings is aiguard's connection to Casdoor, the PDP and OAuth
// authorization server. Editable from the Web UI's "Casdoor connection" page.
type CasdoorSettings struct {
	Endpoint     string `yaml:"endpoint" json:"endpoint"`
	ClientId     string `yaml:"clientId" json:"clientId"`
	ClientSecret string `yaml:"clientSecret" json:"clientSecret"`
	Organization string `yaml:"organization" json:"organization"`
	Application  string `yaml:"application" json:"application"`
}

// InterceptSettings controls the transparent proxy engine. Editable from the
// Web UI's "Interception" page.
type InterceptSettings struct {
	ProxyPort               int    `yaml:"proxyPort" json:"proxyPort"`
	FailClosedOnPdpError    bool   `yaml:"failClosedOnPdpError" json:"failClosedOnPdpError"`
	PassthroughUnrecognized bool   `yaml:"passthroughUnrecognized" json:"passthroughUnrecognized"`
	StepUpDefaultAction     string `yaml:"stepUpDefaultAction" json:"stepUpDefaultAction"`
}

// LLMProvider is one saved LLM API profile (a base URL + API key pair, plus
// optional model overrides), editable from the Web UI's "LLM Providers" page.
// Ids are generated client-side and are otherwise opaque to aiguard.
//
// ApiKey never travels server -> client (see MarshalJSON). Client -> server it
// only carries a value when the operator is setting a new key, so a blank one
// means "no change", not "clear it" - see LLMSettings.PreserveApiKeys.
type LLMProvider struct {
	Id             string `yaml:"id" json:"id"`
	Name           string `yaml:"name" json:"name"`
	BaseUrl        string `yaml:"baseUrl" json:"baseUrl"`
	ApiKey         string `yaml:"apiKey" json:"-"`
	Model          string `yaml:"model,omitempty" json:"model,omitempty"`
	SmallFastModel string `yaml:"smallFastModel,omitempty" json:"smallFastModel,omitempty"`
}

// llmProviderWire is LLMProvider's JSON wire shape, so MarshalJSON/UnmarshalJSON
// have something to delegate the ordinary fields to. LLMProvider itself keeps
// `json:"-"` on ApiKey so a stray json.Marshal elsewhere cannot leak it.
type llmProviderWire struct {
	Id             string `json:"id"`
	Name           string `json:"name"`
	BaseUrl        string `json:"baseUrl"`
	ApiKey         string `json:"apiKey,omitempty"`
	HasApiKey      bool   `json:"hasApiKey"`
	Model          string `json:"model,omitempty"`
	SmallFastModel string `json:"smallFastModel,omitempty"`
}

// MarshalJSON reports whether a key is saved without ever revealing it: the
// browser only ever needs to know "is one configured", not what it is.
func (p LLMProvider) MarshalJSON() ([]byte, error) {
	return json.Marshal(llmProviderWire{
		Id:             p.Id,
		Name:           p.Name,
		BaseUrl:        p.BaseUrl,
		HasApiKey:      p.ApiKey != "",
		Model:          p.Model,
		SmallFastModel: p.SmallFastModel,
	})
}

// UnmarshalJSON accepts apiKey when the operator is providing a new one. An
// absent or empty apiKey means "no change"; PreserveApiKeys does the merging.
func (p *LLMProvider) UnmarshalJSON(data []byte) error {
	var wire llmProviderWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	p.Id = wire.Id
	p.Name = wire.Name
	p.BaseUrl = wire.BaseUrl
	p.ApiKey = wire.ApiKey
	p.Model = wire.Model
	p.SmallFastModel = wire.SmallFastModel
	return nil
}

// LLMActiveProvider records which saved provider one agent installation is
// switched to. Keyed by the same (agentId, path, owner) triple as patch.Target
// rather than agentId alone, because one agent kind can have several
// installations on a host that each need their own active provider.
type LLMActiveProvider struct {
	AgentId    string `yaml:"agentId" json:"agentId"`
	Path       string `yaml:"path" json:"path"`
	Owner      string `yaml:"owner" json:"owner"`
	ProviderId string `yaml:"providerId" json:"providerId"`
}

// LLMSettings is the saved LLM provider profiles and which one is active on
// each patched installation. Editable from the Web UI's "LLM Providers" page
// and the "Provider" column on the Agents page.
type LLMSettings struct {
	Providers []LLMProvider       `yaml:"providers" json:"providers"`
	Active    []LLMActiveProvider `yaml:"active" json:"active"`
}

// PreserveApiKeys fills in an empty ApiKey on each of s.Providers from the
// matching provider in previous, so a client update - which never carries a
// key back down from a GET - does not read as "clear the key". A provider not
// in previous is brand new and keeps whatever the client supplied.
func (s *LLMSettings) PreserveApiKeys(previous LLMSettings) {
	for i, provider := range s.Providers {
		if provider.ApiKey != "" {
			continue
		}
		if old, ok := previous.ProviderById(provider.Id); ok {
			s.Providers[i].ApiKey = old.ApiKey
		}
	}
}

// ValidateProviderRemoval rejects an update that would drop a provider still
// referenced by an Active entry. Deleting one out from under an agent orphans
// that installation: its live config keeps the endpoint/key the deleted
// provider last wrote, with nothing left in the UI to explain where it came
// from. The operator must switch the agent off it first.
func (s *LLMSettings) ValidateProviderRemoval(next LLMSettings) error {
	for _, active := range s.Active {
		if active.ProviderId == "" {
			continue
		}
		if _, stillExists := next.ProviderById(active.ProviderId); stillExists {
			continue
		}
		name := active.ProviderId
		if provider, ok := s.ProviderById(active.ProviderId); ok {
			name = provider.Name
		}
		return fmt.Errorf("cannot delete %q: %s is switched to it - switch it back to System default first", name, active.AgentId)
	}
	return nil
}

// ProviderById looks up a saved provider profile by id.
func (s *LLMSettings) ProviderById(id string) (LLMProvider, bool) {
	for _, provider := range s.Providers {
		if provider.Id == id {
			return provider, true
		}
	}
	return LLMProvider{}, false
}

// ActiveProviderId reports which provider id is active for one installation,
// or "" if it is on the agent's own default.
func (s *LLMSettings) ActiveProviderId(agentId, path, owner string) string {
	for _, active := range s.Active {
		if active.AgentId == agentId && active.Path == path && active.Owner == owner {
			return active.ProviderId
		}
	}
	return ""
}

// SetActive records which provider is now active for one installation,
// upserting by the (agentId, path, owner) triple. Passing an empty
// providerId removes the entry, since "" is also what ActiveProviderId
// returns for an installation with no entry at all.
func (s *LLMSettings) SetActive(agentId, path, owner, providerId string) {
	for i, active := range s.Active {
		if active.AgentId == agentId && active.Path == path && active.Owner == owner {
			if providerId == "" {
				s.Active = append(s.Active[:i], s.Active[i+1:]...)
			} else {
				s.Active[i].ProviderId = providerId
			}
			return
		}
	}
	if providerId != "" {
		s.Active = append(s.Active, LLMActiveProvider{AgentId: agentId, Path: path, Owner: owner, ProviderId: providerId})
	}
}

// Settings is the full set of live, Web-UI-editable runtime settings. It's
// seeded from conf/app.conf on first run, then lives in its own file
// (settings.yaml) so edits made from the UI persist independently of the
// static app.conf bootstrap file.
type Settings struct {
	Casdoor   CasdoorSettings   `yaml:"casdoor" json:"casdoor"`
	Intercept InterceptSettings `yaml:"intercept" json:"intercept"`
	LLM       LLMSettings       `yaml:"llm" json:"llm"`
}

var (
	currentSettings *Settings
	settingsMutex   sync.RWMutex
)

const settingsFileName = "settings.yaml"

func settingsPath() string {
	return filepath.Join(filepath.Dir(conf.GetPolicyFile()), settingsFileName)
}

func settingsFromConf() *Settings {
	return &Settings{
		Casdoor: CasdoorSettings{
			Endpoint:     conf.GetCasdoorEndpoint(),
			ClientId:     conf.GetCasdoorClientId(),
			ClientSecret: conf.GetCasdoorClientSecret(),
			Organization: conf.GetCasdoorOrganization(),
			Application:  conf.GetCasdoorApplication(),
		},
		Intercept: InterceptSettings{
			ProxyPort:               conf.GetProxyPort(),
			FailClosedOnPdpError:    conf.FailClosedOnPdpError(),
			PassthroughUnrecognized: conf.PassthroughUnrecognized(),
			StepUpDefaultAction:     conf.GetStepUpDefaultAction(),
		},
		LLM: LLMSettings{Providers: []LLMProvider{}, Active: []LLMActiveProvider{}},
	}
}

// normalizeLLM replaces nil Providers/Active with empty slices, so the Web UI
// always sees "[]" rather than "null" whether settings.yaml predates the LLM
// section or simply never saved any providers.
func (s *Settings) normalizeLLM() {
	if s.LLM.Providers == nil {
		s.LLM.Providers = []LLMProvider{}
	}
	if s.LLM.Active == nil {
		s.LLM.Active = []LLMActiveProvider{}
	}
}

// InitSettings loads settings.yaml, seeding it from conf/app.conf if it
// doesn't exist yet. Call once at startup.
func InitSettings() error {
	path := settingsPath()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		s := settingsFromConf()
		SetSettings(s)
		return SaveSettings(s)
	} else if err != nil {
		return err
	}

	var s Settings
	if err := yaml.Unmarshal(data, &s); err != nil {
		return err
	}
	SetSettings(&s)
	return nil
}

// SaveSettings writes settings to settings.yaml and updates the in-memory copy.
func SaveSettings(s *Settings) error {
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	if err := util.AtomicWriteFile(settingsPath(), data, 0o600); err != nil {
		return err
	}
	SetSettings(s)
	return nil
}

func SetSettings(s *Settings) {
	s.normalizeLLM()
	settingsMutex.Lock()
	defer settingsMutex.Unlock()
	currentSettings = s
}

// GetSettings returns a copy of the live settings, falling back to
// conf/app.conf defaults if InitSettings hasn't run yet (e.g. in unit tests).
// It never hands back the live object itself: a caller mutating a field on
// what GetSettings returned would race every other caller doing the same.
// MutateSettings is the one place allowed to change and persist a copy.
func GetSettings() *Settings {
	settingsMutex.RLock()
	defer settingsMutex.RUnlock()
	if currentSettings == nil {
		return settingsFromConf()
	}
	return currentSettings.clone()
}

// clone deep-copies the parts of Settings a caller could mutate in place -
// today just the LLM slices. CasdoorSettings and InterceptSettings hold no
// slices or maps, so the top-level struct copy already isolates them.
func (s *Settings) clone() *Settings {
	clone := *s
	// make + append, not append(nil, ...): appending zero elements onto a nil
	// slice returns nil, undoing normalizeLLM and handing the Web UI a JSON
	// "null" where it expects "[]".
	clone.LLM.Providers = append(make([]LLMProvider, 0, len(s.LLM.Providers)), s.LLM.Providers...)
	clone.LLM.Active = append(make([]LLMActiveProvider, 0, len(s.LLM.Active)), s.LLM.Active...)
	return &clone
}

// settingsUpdateMutex serializes MutateSettings calls so two concurrent
// read-modify-write sequences cannot both read the same starting state and
// each persist a version that discards the other's change.
var settingsUpdateMutex sync.Mutex

// MutateSettings turns "read the current settings" into "persist a changed
// version" as one atomic step. build receives a private copy, safe to mutate,
// and returns the settings to persist. A non-nil error aborts without
// persisting anything.
func MutateSettings(build func(current *Settings) (*Settings, error)) (*Settings, error) {
	settingsUpdateMutex.Lock()
	defer settingsUpdateMutex.Unlock()

	next, err := build(GetSettings())
	if err != nil {
		return nil, err
	}
	if err := SaveSettings(next); err != nil {
		return nil, err
	}
	return next, nil
}
