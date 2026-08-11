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

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestClaudeDesktopLLMSwitchSupportedMatchesGOOS pins claude_desktop_llm.go's
// platform gate to the same boundary claude_desktop.go's Patch already uses
// for sharing hooks: Windows only.
func TestClaudeDesktopLLMSwitchSupportedMatchesGOOS(t *testing.T) {
	want := runtime.GOOS == "windows"
	if got := LLMSwitchSupported("claude-desktop"); got != want {
		t.Errorf("LLMSwitchSupported(%q) = %v, want %v (GOOS=%s)", "claude-desktop", got, want, runtime.GOOS)
	}
}

func TestClaudeDesktopLLMSwitchRejectedOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this platform is where claude-desktop LLM switching is supported; see the Windows round-trip test instead")
	}
	target := Target{AgentId: "claude-desktop", Path: "/usr/local/bin/claude-desktop"}
	if err := ApplyProvider(target, LLMProvider{BaseUrl: "https://api.example.com", ApiKey: "sk-test"}); err == nil {
		t.Error("ApplyProvider should refuse claude-desktop outside Windows")
	}
	if err := ClearProvider(target); err == nil {
		t.Error("ClearProvider should refuse claude-desktop outside Windows")
	}
}

// TestClaudeDesktopLLMSwitchRoundTripOnWindows pins that Desktop's Code tab is
// edited through the identical ~/.claude/settings.json path claude-code uses.
func TestClaudeDesktopLLMSwitchRoundTripOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("claude-desktop LLM switching is only supported on Windows")
	}

	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)

	configPath := filepath.Join(home, ".claude", "settings.json")
	target := Target{AgentId: "claude-desktop", Path: `C:\Users\test\AppData\Local\AnthropicClaude\claude.exe`}

	if err := ApplyProvider(target, LLMProvider{BaseUrl: "https://api.example.com/v1", ApiKey: "sk-test-123"}); err != nil {
		t.Fatalf("ApplyProvider: %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	env, _ := config["env"].(map[string]any)
	if env["ANTHROPIC_BASE_URL"] != "https://api.example.com/v1" {
		t.Errorf("ANTHROPIC_BASE_URL = %v, want https://api.example.com/v1", env["ANTHROPIC_BASE_URL"])
	}

	if err := ClearProvider(target); err != nil {
		t.Fatalf("ClearProvider: %v", err)
	}
	data, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	config = nil
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if _, has := config["env"]; has {
		t.Errorf("env block should be removed once empty after ClearProvider, got %v", config["env"])
	}
}
