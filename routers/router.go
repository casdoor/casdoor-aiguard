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

package routers

import (
	"github.com/beego/beego/v2/server/web"
	"github.com/casdoor/casdoor-aiguard/controllers"
)

func InitAPI() {
	web.Router("/api/agents", &controllers.ApiController{}, "GET:GetAgents")
	web.Router("/api/agents/patch", &controllers.ApiController{}, "POST:PatchAgent")
	web.Router("/api/agents/unpatch", &controllers.ApiController{}, "POST:UnpatchAgent")
	web.Router("/api/events", &controllers.ApiController{}, "GET:GetEvents")
	web.Router("/api/records", &controllers.ApiController{}, "GET:GetRecords")
	web.Router("/api/records", &controllers.ApiController{}, "POST:AddRecord")
	web.Router("/api/enforce", &controllers.ApiController{}, "POST:Enforce")
	web.Router("/api/policy", &controllers.ApiController{}, "GET:GetPolicy")
	web.Router("/api/policy", &controllers.ApiController{}, "POST:UpdatePolicy")
	web.Router("/api/policy-sets", &controllers.ApiController{}, "GET:GetPolicySets")
	web.Router("/api/policy-set", &controllers.ApiController{}, "GET:GetPolicySet")
	web.Router("/api/policy-set/enable", &controllers.ApiController{}, "POST:TogglePolicySet")
	web.Router("/api/settings", &controllers.ApiController{}, "GET:GetSettings")
	web.Router("/api/settings", &controllers.ApiController{}, "POST:UpdateSettings")
	web.Router("/api/ca-cert", &controllers.ApiController{}, "GET:DownloadCaCert")
}
