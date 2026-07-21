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

// Package auth wraps the Casdoor SDK for aiguard's *operator* login - the
// person using the Web UI, as opposed to casdoorclient, which authenticates
// aiguard itself to the PDP. Login is entirely optional: when Casdoor is not
// configured or not reachable, the Web UI simply stays anonymous, so every
// call here degrades to "unavailable" instead of failing hard.
package auth

import (
	"fmt"
	"sync"
	"time"

	"github.com/beego/beego/v2/core/logs"
	"github.com/casdoor/casdoor-aiguard/object"
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"golang.org/x/oauth2"
)

// Type aliases so the rest of the codebase only imports this package.
type (
	Claims = casdoorsdk.Claims
	User   = casdoorsdk.User
)

// Config is what the Web UI needs to build a Casdoor sign-in URL itself.
// It deliberately excludes the client secret.
type Config struct {
	Endpoint     string `json:"endpoint"`
	ClientId     string `json:"clientId"`
	Organization string `json:"organization"`
	Application  string `json:"application"`
	// Available is false until aiguard has fetched the application's signing
	// certificate from Casdoor; the UI hides the login button while it is.
	Available bool `json:"available"`
}

var (
	configMutex sync.RWMutex
	available   bool
)

// GetConfig returns the current sign-in configuration for the Web UI.
func GetConfig() *Config {
	casdoor := object.GetSettings().Casdoor

	configMutex.RLock()
	defer configMutex.RUnlock()

	return &Config{
		Endpoint:     casdoor.Endpoint,
		ClientId:     casdoor.ClientId,
		Organization: casdoor.Organization,
		Application:  casdoor.Application,
		Available:    available,
	}
}

func IsAvailable() bool {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return available
}

func setAvailable(v bool) {
	configMutex.Lock()
	defer configMutex.Unlock()
	available = v
}

// tryInitConfig initialises the SDK from the live settings. The application's
// signing certificate is not part of the settings, so it takes two passes: a
// bare client-credentials config to read the application, then a full config
// that can verify the JWTs Casdoor hands back after login.
func tryInitConfig() error {
	casdoor := object.GetSettings().Casdoor
	if casdoor.Endpoint == "" || casdoor.ClientId == "" || casdoor.ClientSecret == "" {
		return fmt.Errorf("casdoor endpoint, clientId or clientSecret is not configured")
	}

	casdoorsdk.InitConfig(casdoor.Endpoint, casdoor.ClientId, casdoor.ClientSecret, "", casdoor.Organization, casdoor.Application)

	application, err := casdoorsdk.GetApplication(casdoor.Application)
	if err != nil {
		return err
	}
	if application == nil {
		return fmt.Errorf("the application %q does not exist", casdoor.Application)
	}

	cert, err := casdoorsdk.GetCert(application.Cert)
	if err != nil {
		return err
	}
	if cert == nil {
		return fmt.Errorf("the cert %q does not exist", application.Cert)
	}

	casdoorsdk.InitConfig(casdoor.Endpoint, casdoor.ClientId, casdoor.ClientSecret, cert.Certificate, casdoor.Organization, casdoor.Application)
	setAvailable(true)
	return nil
}

// InitConfig configures operator login from the live settings. Casdoor being
// down or unconfigured is not fatal - it only means the Web UI stays
// anonymous - so failures are logged and retried in the background until the
// connection comes up (or until another InitConfig call supersedes this one,
// e.g. after the operator edits the Casdoor Connection page).
func InitConfig() {
	err := tryInitConfig()
	if err == nil {
		logs.Info("auth: casdoor login is ready")
		return
	}

	setAvailable(false)
	logs.Warn("auth: casdoor login unavailable, retrying in background: %v", err)

	generation := startGeneration()
	go func() {
		for {
			time.Sleep(10 * time.Second)
			if !isCurrentGeneration(generation) {
				return // a newer InitConfig call took over
			}
			if err := tryInitConfig(); err != nil {
				setAvailable(false)
				logs.Warn("auth: casdoor login still unavailable: %v", err)
			} else {
				logs.Info("auth: casdoor login is ready")
				return
			}
		}
	}()
}

// Only the newest InitConfig call owns the retry loop, so that re-configuring
// from the Web UI does not leave older loops racing to install stale settings.
var (
	generationMutex   sync.Mutex
	currentGeneration int
)

func startGeneration() int {
	generationMutex.Lock()
	defer generationMutex.Unlock()
	currentGeneration++
	return currentGeneration
}

func isCurrentGeneration(generation int) bool {
	generationMutex.Lock()
	defer generationMutex.Unlock()
	return generation == currentGeneration
}

func GetOAuthToken(code string, state string) (*oauth2.Token, error) {
	return casdoorsdk.GetOAuthToken(code, state)
}

func ParseJwtToken(token string) (*Claims, error) {
	return casdoorsdk.ParseJwtToken(token)
}

func GetUser(name string) (*User, error) {
	return casdoorsdk.GetUser(name)
}

// UpdateUserProperties writes back only the user's "properties" column, so
// saving something aiguard owns (a digital employee's policy set) can never
// overwrite a field of the user that Casdoor, or another client, changed in
// the meantime.
func UpdateUserProperties(user *User) error {
	affected, err := casdoorsdk.UpdateUserForColumns(user, []string{"properties"})
	if err != nil {
		return err
	}
	if !affected {
		return fmt.Errorf("casdoor did not accept the update of the user %q", user.Name)
	}
	return nil
}
