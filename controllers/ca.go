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

package controllers

import (
	"github.com/casdoor/casdoor-aiguard/proxy"
)

// DownloadCaCert
// @Title DownloadCaCert
// @Description download aiguard's local CA certificate (PEM), to be trusted
// @Description on any host or base image running agents aiguard intercepts.
// @router /ca-cert [get]
func (c *ApiController) DownloadCaCert() {
	path := proxy.CACertPath()
	c.Ctx.Output.Header("Content-Disposition", "attachment; filename=aiguard-ca.crt")
	c.Ctx.Output.Header("Content-Type", "application/x-pem-file")
	c.Ctx.Output.Download(path)
}
