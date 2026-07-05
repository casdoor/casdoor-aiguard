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

package conf

import (
	"os"
	"strconv"

	"github.com/beego/beego/v2/server/web"
)

func init() {
	presetConfigItems := []string{"httpport", "appname"}
	for _, key := range presetConfigItems {
		if value, ok := os.LookupEnv(key); ok {
			err := web.AppConfig.Set(key, value)
			if err != nil {
				panic(err)
			}
		}
	}
}

func GetConfigString(key string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	res, _ := web.AppConfig.String(key)
	return res
}

func GetConfigBool(key string) bool {
	return GetConfigString(key) == "true"
}

func GetConfigInt(key string, defaultValue int) int {
	value := GetConfigString(key)
	if value == "" {
		return defaultValue
	}
	num, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return num
}

func GetCasdoorEndpoint() string {
	return GetConfigString("casdoorEndpoint")
}

func GetCasdoorClientId() string {
	return GetConfigString("casdoorClientId")
}

func GetCasdoorClientSecret() string {
	return GetConfigString("casdoorClientSecret")
}

func GetCasdoorCertificate() string {
	return GetConfigString("casdoorCertificate")
}

func GetCasdoorOrganization() string {
	return GetConfigString("casdoorOrganization")
}

func GetCasdoorApplication() string {
	return GetConfigString("casdoorApplication")
}

func GetProxyPort() int {
	return GetConfigInt("proxyPort", 9090)
}

func GetCaCertDir() string {
	dir := GetConfigString("caCertDir")
	if dir == "" {
		dir = "./certs"
	}
	return dir
}

func GetPolicyFile() string {
	file := GetConfigString("policyFile")
	if file == "" {
		file = "./conf/policy.yaml"
	}
	return file
}

func GetAuditLogFile() string {
	file := GetConfigString("auditLogFile")
	if file == "" {
		file = "./logs/audit.log"
	}
	return file
}

// FailClosedOnPdpError controls behavior for recognized sensitive operations
// when Casdoor (the PDP) is unreachable: true means deny by default.
func FailClosedOnPdpError() bool {
	value := GetConfigString("failClosedOnPdpError")
	if value == "" {
		return true
	}
	return value == "true"
}

// PassthroughUnrecognized controls behavior for traffic that no recognizer
// could identify: true means let it through untouched.
func PassthroughUnrecognized() bool {
	value := GetConfigString("passthroughUnrecognized")
	if value == "" {
		return true
	}
	return value == "true"
}

// GetStepUpDefaultAction is the verdict a step-up (CIBA) decision resolves to
// while CIBA is stubbed in stage 1: "deny" (safe default) or "allow".
func GetStepUpDefaultAction() string {
	value := GetConfigString("stepUpDefaultAction")
	if value == "" {
		return "deny"
	}
	return value
}

// GetPdpEnforcePath is the Casdoor endpoint path used for enforcement decisions.
func GetPdpEnforcePath() string {
	path := GetConfigString("pdpEnforcePath")
	if path == "" {
		path = "/api/enforce"
	}
	return path
}
