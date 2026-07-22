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
	web.Router("/api/auth-config", &controllers.ApiController{}, "GET:GetAuthConfig")
	web.Router("/api/signin", &controllers.ApiController{}, "POST:Signin")
	web.Router("/api/signout", &controllers.ApiController{}, "POST:Signout")
	web.Router("/api/account", &controllers.ApiController{}, "GET:GetAccount")
	web.Router("/api/host-info", &controllers.ApiController{}, "GET:GetHostInfo")
	web.Router("/api/agents", &controllers.ApiController{}, "GET:GetAgents")
	web.Router("/api/agents/patch", &controllers.ApiController{}, "POST:PatchAgent")
	web.Router("/api/agents/unpatch", &controllers.ApiController{}, "POST:UnpatchAgent")
	web.Router("/api/events", &controllers.ApiController{}, "GET:GetEvents")
	web.Router("/api/records", &controllers.ApiController{}, "GET:GetRecords")
	web.Router("/api/records", &controllers.ApiController{}, "POST:AddRecord")
	web.Router("/api/records/feedback", &controllers.ApiController{}, "POST:UpdateRecordFeedback")
	web.Router("/api/enforce", &controllers.ApiController{}, "POST:Enforce")
	web.Router("/api/policy", &controllers.ApiController{}, "GET:GetPolicy")
	web.Router("/api/policy", &controllers.ApiController{}, "POST:UpdatePolicy")
	web.Router("/api/policy-sets", &controllers.ApiController{}, "GET:GetPolicySets")
	web.Router("/api/policy-set", &controllers.ApiController{}, "GET:GetPolicySet")
	web.Router("/api/policy-set/enable", &controllers.ApiController{}, "POST:TogglePolicySet")
	web.Router("/api/employee-policy-set", &controllers.ApiController{}, "GET:GetEmployeePolicySet")
	web.Router("/api/employee-policy-set", &controllers.ApiController{}, "POST:UpdateEmployeePolicySet")
	web.Router("/api/learned-policy-set", &controllers.ApiController{}, "GET:GetLearnedPolicySet")
	web.Router("/api/learned-policy-set/delete", &controllers.ApiController{}, "POST:DeleteLearnedRule")
	web.Router("/api/settings", &controllers.ApiController{}, "GET:GetSettings")
	web.Router("/api/settings", &controllers.ApiController{}, "POST:UpdateSettings")
	web.Router("/api/ca-cert", &controllers.ApiController{}, "GET:DownloadCaCert")
}
