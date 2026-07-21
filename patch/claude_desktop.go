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
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/casdoor/casdoor-aiguard/conf"
	"github.com/casdoor/casdoor-aiguard/mcpserver"
)

// Claude Desktop is the hardest of the agents to instrument, and the shape of
// the patch follows from that. It is a packaged application: no hook directory,
// no event scripts, no CLI to drive. The one extension point its user can
// configure is the MCP server list in claude_desktop_config.json, so that is
// what the patch writes to - registering the aiguard binary itself as an MCP
// server, which the app then launches over stdio (see package mcpserver).
//
// Two consequences worth being clear about. The app reads that file only at
// launch, so a patch takes effect on the next restart and there is no command
// aiguard can run to hurry it along. And an MCP server sees only the traffic
// addressed to it: the records this yields are the session handshake and the
// tool calls the model routes through aiguard, not the app's conversations.
// Interception (the Events page) remains the way to see the rest.

const claudeDesktopServerName = "casdoor-aiguard"

func init() {
	register(claudeDesktopPatcher{})
}

type claudeDesktopPatcher struct{}

func (claudeDesktopPatcher) AgentId() string { return "claude-desktop" }

func (claudeDesktopPatcher) Supported() bool { return true }

func (p claudeDesktopPatcher) Patch(target Target) error {
	configPath, err := p.configPath(target)
	if err != nil {
		return err
	}
	entry, err := p.serverEntry()
	if err != nil {
		return err
	}

	return Apply(target, func(changes *ChangeSet) error {
		if err := changes.MkdirAll(filepath.Dir(configPath)); err != nil {
			return err
		}

		config, err := readJSONObject(changes, configPath)
		if err != nil {
			return err
		}
		childObject(config, "mcpServers")[claudeDesktopServerName] = entry

		updated, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return err
		}
		return changes.WriteFile(configPath, append(updated, '\n'), 0o600)
	})
}

func (p claudeDesktopPatcher) Unpatch(target Target) error {
	// Revert restores claude_desktop_config.json to its exact pre-patch bytes,
	// which removes the server entry along with anything else the patch wrote.
	return Revert(target)
}

func (p claudeDesktopPatcher) Status(target Target) (Status, error) {
	configPath, err := p.configPath(target)
	if err != nil {
		return Status{}, err
	}

	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return Status{Detail: "not patched"}, nil
	}
	if err != nil {
		return Status{}, err
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return Status{}, fmt.Errorf("cannot parse %s: %w", configPath, err)
	}

	if _, ok := lookupObject(config, "mcpServers", claudeDesktopServerName); !ok {
		return Status{Detail: "not patched"}, nil
	}
	if !IsApplied(target) {
		return Status{Patched: true, Detail: "patched outside aiguard: unpatch cannot restore the original config"}, nil
	}
	return Status{Patched: true, Detail: "restart Claude Desktop to apply - it reads its config only at launch"}, nil
}

// serverEntry is the MCP server registration Claude Desktop launches. It points
// at this very binary, so instrumenting the app adds no runtime dependency -
// no Node, no separate package to keep in step with aiguard's own version.
func (p claudeDesktopPatcher) serverEntry() (map[string]any, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("cannot resolve the aiguard binary to register: %w", err)
	}
	// The app launches this from its own working directory, so the endpoint has
	// to travel with the command rather than be read from aiguard's app.conf.
	return map[string]any{
		"command": executable,
		"args": []string{
			mcpserver.Subcommand,
			"--agent", p.AgentId(),
			"--records-url", conf.GetRecordsIngestUrl(),
		},
	}, nil
}

// configPath locates claude_desktop_config.json, which lives in the platform's
// per-user application data directory.
func (p claudeDesktopPatcher) configPath(target Target) (string, error) {
	home, err := homeOf(target)
	if err != nil {
		return "", err
	}

	var dir string
	switch runtime.GOOS {
	case "windows":
		dir = filepath.Join(home, "AppData", "Roaming", "Claude")
		if appData := os.Getenv("APPDATA"); appData != "" && isCurrentUser(target.Owner) {
			dir = filepath.Join(appData, "Claude")
		}
	case "darwin":
		dir = filepath.Join(home, "Library", "Application Support", "Claude")
	default:
		dir = filepath.Join(home, ".config", "Claude")
	}
	return filepath.Join(dir, "claude_desktop_config.json"), nil
}

// readJSONObject reads a JSON config the patch is about to edit, treating a
// missing or empty file as an empty object so the patch can create it.
func readJSONObject(changes *ChangeSet, path string) (map[string]any, error) {
	data, err := changes.ReadFile(path)
	if err != nil {
		return nil, err
	}
	config := map[string]any{}
	if len(strings.TrimSpace(string(data))) == 0 {
		return config, nil
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return config, nil
}
