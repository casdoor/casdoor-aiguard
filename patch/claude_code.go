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

// claudeCodePatcher will instrument Claude Code, whose extension point is
// unlike OpenClaw's: instead of a hook directory discovered by a long-running
// gateway, Claude Code reads a "hooks" block in ~/.claude/settings.json that
// names shell commands to run on events such as PreToolUse and PostToolUse, and
// it picks up the file per session rather than needing a restart.
//
// Implementing it means writing Patch to merge that block into settings.json
// through the ChangeSet, Unpatch to call Revert, and Status to report whether
// the block is still there.
type claudeCodePatcher struct {
	unimplemented
}

func init() {
	register(claudeCodePatcher{unimplemented{id: "claude-code"}})
}
