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
	"path/filepath"
	"sync"

	"github.com/casdoor/casdoor-aiguard/conf"
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

// Settings is the full set of live, Web-UI-editable runtime settings. It's
// seeded from conf/app.conf on first run, then lives in its own file
// (settings.yaml) so edits made from the UI persist independently of the
// static app.conf bootstrap file.
type Settings struct {
	Casdoor   CasdoorSettings   `yaml:"casdoor" json:"casdoor"`
	Intercept InterceptSettings `yaml:"intercept" json:"intercept"`
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
	if err := os.WriteFile(settingsPath(), data, 0o600); err != nil {
		return err
	}
	SetSettings(s)
	return nil
}

func SetSettings(s *Settings) {
	settingsMutex.Lock()
	defer settingsMutex.Unlock()
	currentSettings = s
}

// GetSettings returns the live settings, falling back to conf/app.conf
// defaults if InitSettings hasn't run yet (e.g. in unit tests).
func GetSettings() *Settings {
	settingsMutex.RLock()
	defer settingsMutex.RUnlock()
	if currentSettings == nil {
		return settingsFromConf()
	}
	return currentSettings
}
