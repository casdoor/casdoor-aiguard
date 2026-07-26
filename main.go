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

package main

import (
	"fmt"

	"github.com/beego/beego/v2/core/logs"
	"github.com/beego/beego/v2/server/web"
	"github.com/casdoor/casdoor-aiguard/agenthook"
	"github.com/casdoor/casdoor-aiguard/agentmonitor"
	"github.com/casdoor/casdoor-aiguard/auth"
	_ "github.com/casdoor/casdoor-aiguard/conf"
	"github.com/casdoor/casdoor-aiguard/mcpserver"
	"github.com/casdoor/casdoor-aiguard/object"
	"github.com/casdoor/casdoor-aiguard/proxy"
	"github.com/casdoor/casdoor-aiguard/routers"
	"github.com/casdoor/casdoor-aiguard/util"
)

func main() {
	// Records and exits when an agent launched this binary as a command hook.
	agenthook.ServeIfInvoked()

	// Serves and exits when an agent launched this binary as its MCP server.
	mcpserver.ServeIfInvoked()

	util.LogRuntimeEnv()

	if err := object.InitSettings(); err != nil {
		panic(err)
	}
	if err := object.InitPolicy(); err != nil {
		panic(err)
	}
	if err := object.InitPolicySetState(); err != nil {
		panic(err)
	}
	if err := object.InitAuditLog(); err != nil {
		panic(err)
	}
	if err := object.InitRecordLog(); err != nil {
		panic(err)
	}
	if err := agentmonitor.Start(); err != nil {
		logs.Error("agent monitor could not start: %v", err)
	}
	if err := agentmonitor.StartCodexMonitor(); err != nil {
		logs.Error("Codex rollout monitor could not start: %v", err)
	}
	defer agentmonitor.Stop()
	defer agentmonitor.StopCodexMonitor()

	ca, err := proxy.LoadOrCreateCA()
	if err != nil {
		panic(fmt.Sprintf("failed to load/create aiguard CA: %v", err))
	}
	logs.Info("aiguard CA ready at %s - trust it on any host/image running intercepted agents", ca.CertPEMPath())

	engine := proxy.NewEngine(ca)
	go func() {
		if err := engine.Start(); err != nil {
			logs.Error("aiguard proxy engine stopped: %v", err)
		}
	}()

	proxy.ManageTransparentRedirect()

	// Operator login is optional: this only prepares it, and a Casdoor that is
	// unconfigured or down just leaves the Web UI anonymous.
	auth.InitConfig()

	// Sessions hold the signed-in operator. The file provider keeps a login
	// alive across aiguard restarts, which are frequent while patching agents;
	web.BConfig.WebConfig.Session.SessionOn = true
	web.BConfig.WebConfig.Session.SessionName = "aiguard_session_id"
	web.BConfig.WebConfig.Session.SessionProvider = "file"
	web.BConfig.WebConfig.Session.SessionProviderConfig = "./tmp"
	sessionCookieLifeTime := 3600 * 24 * 30
	web.BConfig.WebConfig.Session.SessionCookieLifeTime = sessionCookieLifeTime
	web.BConfig.WebConfig.Session.SessionGCMaxLifetime = int64(sessionCookieLifeTime)

	routers.InitAPI()
	routers.InitStatic()

	port := web.AppConfig.DefaultInt("httpport", 9000)
	logs.Info("aiguard management API listening on :%d", port)
	web.Run(fmt.Sprintf(":%d", port))
}
