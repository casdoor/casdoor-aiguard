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
	"os"
	"runtime"
)

// HostInfo describes the machine this aiguard instance guards. The Web UI
// shows it in the top bar to make the host binding obvious.
type HostInfo struct {
	Hostname string `json:"hostname"`
	Os       string `json:"os"`
	Arch     string `json:"arch"`
}

// GetHostInfo
// @Title GetHostInfo
// @Description get the hostname of the machine aiguard runs on
// @router /host-info [get]
func (c *ApiController) GetHostInfo() {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	c.ResponseOk(&HostInfo{
		Hostname: hostname,
		Os:       runtime.GOOS,
		Arch:     runtime.GOARCH,
	})
}
