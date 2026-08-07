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
	"fmt"
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

// GetAuditLogFile is the legacy append-only log of intercepted-traffic
// events. See GetRecordLogFile's comment - the same applies here.
func GetAuditLogFile() string {
	file := GetConfigString("auditLogFile")
	if file == "" {
		file = "./logs/audit.log"
	}
	return file
}

// GetRecordLogFile is the legacy append-only log of behaviour records
// reported by patched agents, the counterpart to GetAuditLogFile. Both now
// exist only as a one-time import source for GetDatabaseFile - see
// object.InitDatabase - and are no longer written to.
func GetRecordLogFile() string {
	file := GetConfigString("recordLogFile")
	if file == "" {
		file = "./logs/record.log"
	}
	return file
}

// GetDatabaseFile is aiguard's local SQLite database: records and events both
// live here now, replacing the in-memory ring buffer + append-only file each
// used before (see object.InitDatabase). A real database persists correctly
// by construction, so restarting no longer needs any of the "replay the log
// back into memory" logic that split ever needed.
func GetDatabaseFile() string {
	file := GetConfigString("databaseFile")
	if file == "" {
		file = "./data/aiguard.db"
	}
	return file
}

// GetPatchStateDir holds what aiguard needs to undo a patch: one manifest per
// patched agent plus the backups of every file the patch overwrote. Losing it
// means losing the ability to unpatch cleanly.
func GetPatchStateDir() string {
	dir := GetConfigString("patchStateDir")
	if dir == "" {
		dir = "./data/patches"
	}
	return dir
}

// GetRecordsIngestUrl is the endpoint aiguard bakes into the hooks it installs
// into agents. It defaults to this process's own management API on the loopback
// address, which is right whenever the agent runs alongside aiguard; an agent
// inside a container or WSL reaches the host by another address, so configure it
// explicitly there.
func GetRecordsIngestUrl() string {
	url := GetConfigString("recordsIngestUrl")
	if url == "" {
		url = fmt.Sprintf("http://127.0.0.1:%d/api/records", GetConfigInt("httpport", 9000))
	}
	return url
}

// GetEnforceUrl is the endpoint a patched agent's intercept path calls to ask
// aiguard for a verdict before performing an operation. It sits beside the
// records ingest URL and defaults the same way: this process's own management
// API on loopback, overridable for an agent that reaches the host by another
// address.
func GetEnforceUrl() string {
	url := GetConfigString("enforceUrl")
	if url == "" {
		url = fmt.Sprintf("http://127.0.0.1:%d/api/enforce", GetConfigInt("httpport", 9000))
	}
	return url
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

// AutoTransparentProxy controls whether aiguard installs the iptables
// transparent redirect on startup and removes it on shutdown. It only takes
// effect on Linux running as root; elsewhere it is ignored and aiguard serves
// as an explicit forward proxy instead. Defaults to true.
func AutoTransparentProxy() bool {
	value := GetConfigString("autoTransparentProxy")
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

// GetPolicyHubDir is the directory holding the Policy Hub's policy sets, one
// JSON file per set. It is read at request time so sets can be added, edited or
// removed without restarting aiguard.
func GetPolicyHubDir() string {
	dir := GetConfigString("policyHubDir")
	if dir == "" {
		dir = "./data/policyhub"
	}
	return dir
}
