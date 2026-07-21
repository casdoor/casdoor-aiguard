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

package patch

// The agents aiguard discovers but cannot instrument yet. Each is registered so
// the Web UI can say "not implemented yet" for an installation it found, rather
// than the far more alarming "no patcher registered for this agent". The note
// on each records where its extension point is, which is the first thing anyone
// writing the real patcher needs.
func init() {
	for _, id := range []string{
		// Reads ~/.codex/config.toml, whose [mcp_servers] table registers MCP
		// servers and whose notify key names a program to run per turn.
		"codex-cli",
		// A VS Code fork: settings live in the editor's User/settings.json and
		// MCP servers in the workspace or user mcp.json.
		"cursor",
		// Reads ~/.cursor/cli-config.json alongside the editor's own state.
		"cursor-agent",
		// A VS Code fork like Cursor, with its own ~/.codeium state directory.
		"windsurf",
	} {
		register(unimplemented{id: id})
	}
}
