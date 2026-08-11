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

import "runtime"

// Claude Desktop's Code tab shares Claude Code CLI's user-level
// ~/.claude/settings.json, but claude_desktop.go's Patch only installs the
// shared hooks there under a runtime.GOOS == "windows" guard - elsewhere,
// Desktop's Code tab either doesn't exist or doesn't read this file. LLM
// provider switching follows the same boundary: on Windows it edits the
// exact same file (and the exact same "env" keys) claude-code does; on
// every other OS there is nothing to switch.
type claudeDesktopLLMSwitcher struct{}

func init() { registerLLMSwitcher(claudeDesktopLLMSwitcher{}) }

func (claudeDesktopLLMSwitcher) AgentId() string { return "claude-desktop" }

func (claudeDesktopLLMSwitcher) Supported() bool { return runtime.GOOS == "windows" }

func (claudeDesktopLLMSwitcher) ApplyProvider(target Target, provider LLMProvider) error {
	return updateClaudeCodeLLMEnv(target, claudeCodeLLMEnvValues(provider))
}

func (claudeDesktopLLMSwitcher) ClearProvider(target Target) error {
	return updateClaudeCodeLLMEnv(target, nil)
}
