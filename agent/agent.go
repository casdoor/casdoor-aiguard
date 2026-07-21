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

// Package agent discovers AI agents installed on the host and identifies
// running agent executables. Discovery is intentionally bounded to known,
// verifiable installation layouts rather than walking the whole filesystem.
// Which agents are recognized, and by what paths and command lines, is data:
// see fingerprint.go. The scanners here and in scan_<platform>.go hold only the
// logic shared by every agent.
package agent

// Installation describes one AI agent installation found on the host. AgentId
// is the fingerprint that matched, so callers that act on an installation - the
// patch package instrumenting it, for one - can tell which agent they are
// looking at without re-matching the display name.
type Installation struct {
	AgentId       string `json:"agentId"`
	Name          string `json:"name"`
	Version       string `json:"version,omitempty"`
	Path          string `json:"path"`
	InstallMethod string `json:"installMethod"`
	Owner         string `json:"owner"`
}
