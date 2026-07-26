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

package agent

var codexFingerprints = []Fingerprint{
	{
		ID:          "codex",
		DisplayName: "ChatGPT Desktop (Codex)",
		ExecName:    "ChatGPT",
		MSIXFamily:  "OpenAI.Codex_2p2nqsd0c76g0",
		ExtraExecRules: []PathRule{
			{Suffix: "/chatgpt.app/contents/macos/chatgpt"},
			{Suffix: "/codex.app/contents/macos/codex"},
			{Contains: []string{"/windowsapps/openai.codex_", "__2p2nqsd0c76g0/app/"}, Suffix: "/codex.exe"},
		},
	},
	{
		ID:              "codex-cli",
		DisplayName:     "Codex CLI",
		ExecName:        "codex",
		StateDir:        "codex",
		NpmPackage:      "@openai/codex",
		HomebrewCasks:   []string{"codex", "codex@latest"},
		WindowsUserDirs: []string{"OpenAI/Codex/bin"},
		ExtraExecRules: []PathRule{
			{Exact: "/usr/bin/codex"},
			{Exact: "/usr/local/bin/codex"},
		},
	},
}
