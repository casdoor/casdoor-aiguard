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

// claudeDesktopPatcher will instrument Claude Desktop, the hardest of the three:
// it is a packaged desktop application with no hook directory and no event
// scripts, so its only configurable extension point is the MCP server list in
// claude_desktop_config.json. Recording behaviour there means registering an
// aiguard MCP server and seeing only the tool traffic that flows through it,
// which is a narrower view than the other agents give.
//
// Implementing it means writing Patch to add that server entry through the
// ChangeSet, Unpatch to call Revert, and Status to report whether the entry is
// still present. Note the app reads its config at launch, so a patch only takes
// effect after the user restarts Claude Desktop.
type claudeDesktopPatcher struct {
	unimplemented
}

func init() {
	register(claudeDesktopPatcher{unimplemented{id: "claude-desktop"}})
}
