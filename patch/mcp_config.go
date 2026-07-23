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
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/casdoor/casdoor-aiguard/conf"
	"github.com/casdoor/casdoor-aiguard/mcpserver"
)

// mcpConfigPatcher instruments agents that load stdio MCP servers from a JSON
// config file. Supporting another such agent is one profile entry, not another
// patcher implementation.
type mcpConfigPatcher struct {
	agentId    string
	serverName string
	configFile string
}

var mcpConfigProfiles = []mcpConfigPatcher{{
	agentId:    "claude-desktop",
	serverName: "casdoor-aiguard",
	configFile: "Claude/claude_desktop_config.json",
}}

func init() {
	for _, profile := range mcpConfigProfiles {
		register(profile)
	}
}

func (p mcpConfigPatcher) AgentId() string { return p.agentId }

func (mcpConfigPatcher) Supported() bool {
	return runtime.GOOS == "darwin" || runtime.GOOS == "windows"
}

func (p mcpConfigPatcher) Patch(target Target) error {
	configPath, err := p.path(target)
	if err != nil {
		return err
	}
	config, mode, _, err := readJSONConfig(configPath)
	if err != nil {
		return err
	}
	rawServers, exists := config["mcpServers"]
	servers, ok := objectValue(rawServers)
	if exists && !ok {
		return fmt.Errorf("mcpServers must be a JSON object")
	}
	if !exists {
		servers = map[string]any{}
		config["mcpServers"] = servers
	}
	entry, err := p.entry()
	if err != nil {
		return err
	}
	if p.current(servers[p.serverName], entry) {
		return setTargetFileOwner(configPath, target)
	}

	servers[p.serverName] = entry
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	if err := writeJSONConfig(configPath, config, mode); err != nil {
		return err
	}
	return setTargetFileOwner(configPath, target)
}

func (p mcpConfigPatcher) Unpatch(target Target) error {
	configPath, err := p.path(target)
	if err != nil {
		return err
	}
	config, mode, exists, err := readJSONConfig(configPath)
	if err != nil || !exists {
		return err
	}
	servers, ok := objectValue(config["mcpServers"])
	if !ok || !p.owns(servers[p.serverName]) {
		return nil
	}
	delete(servers, p.serverName)
	if len(servers) == 0 {
		delete(config, "mcpServers")
	}
	if err := writeJSONConfig(configPath, config, mode); err != nil {
		return err
	}
	return setTargetFileOwner(configPath, target)
}

func (p mcpConfigPatcher) Status(target Target) (Status, error) {
	configPath, err := p.path(target)
	if err != nil {
		return Status{}, err
	}
	config, _, exists, err := readJSONConfig(configPath)
	if err != nil {
		return Status{}, err
	}
	if !exists {
		return Status{Detail: "not patched"}, nil
	}
	servers, ok := objectValue(config["mcpServers"])
	if !ok || !p.owns(servers[p.serverName]) {
		return Status{Detail: "not patched"}, nil
	}
	entry, err := p.entry()
	if err != nil {
		return Status{}, err
	}
	if !p.current(servers[p.serverName], entry) {
		return Status{Detail: "MCP server registration needs refresh"}, nil
	}
	return Status{Patched: true, Detail: "MCP audit active; restart the agent after configuration changes"}, nil
}

func (p mcpConfigPatcher) path(target Target) (string, error) {
	if p.configFile == "" {
		return "", fmt.Errorf("%s MCP config file is not defined", p.agentId)
	}
	configDir, err := userConfigDir(target)
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, filepath.FromSlash(p.configFile)), nil
}

func (p mcpConfigPatcher) entry() (map[string]any, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("cannot resolve the aiguard binary to register: %w", err)
	}
	return map[string]any{
		"command": executable,
		"args": []string{
			mcpserver.Subcommand,
			"--agent", p.agentId,
			"--records-url", conf.GetRecordsIngestUrl(),
			"--enforce-url", conf.GetEnforceUrl(),
		},
	}, nil
}

func (p mcpConfigPatcher) owns(value any) bool {
	entry, ok := objectValue(value)
	if !ok {
		return false
	}
	arguments := stringArrayValue(entry["args"])
	agentFlag := slices.Index(arguments, "--agent")
	return len(arguments) > 0 && arguments[0] == mcpserver.Subcommand &&
		agentFlag >= 0 && agentFlag+1 < len(arguments) && arguments[agentFlag+1] == p.agentId
}

func (p mcpConfigPatcher) current(value any, desired map[string]any) bool {
	entry, ok := objectValue(value)
	if !ok || !p.owns(entry) {
		return false
	}
	command, _ := entry["command"].(string)
	desiredCommand, _ := desired["command"].(string)
	return command == desiredCommand &&
		slices.Equal(stringArrayValue(entry["args"]), stringArrayValue(desired["args"]))
}
