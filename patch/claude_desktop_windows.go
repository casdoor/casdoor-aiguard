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

//go:build windows

package patch

import (
	"fmt"
	"os"

	"github.com/casdoor/casdoor-aiguard/agent"
	"github.com/casdoor/casdoor-aiguard/agentmonitor"
)

func init() {
	register(claudeDesktopPatcher{})
}

// Patch enables online collection from the selected user's Cowork audit logs.
// It does not modify Claude Desktop or its configuration.
type claudeDesktopPatcher struct{}

func (claudeDesktopPatcher) AgentId() string { return "claude-desktop" }

func (claudeDesktopPatcher) Supported() bool { return true }

func (claudeDesktopPatcher) Patch(target Target) error {
	info, err := os.Stat(target.Path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || agent.IdentifyExecutable(target.Path) != "claude-desktop" {
		return fmt.Errorf("%s is not a recognized Claude Desktop executable", target.Path)
	}
	return agentmonitor.Enable(target.Path, target.Owner)
}

func (claudeDesktopPatcher) Unpatch(target Target) error {
	return agentmonitor.Disable(target.Path)
}

func (claudeDesktopPatcher) Status(target Target) (Status, error) {
	enabled, detail := agentmonitor.Status(target.Path)
	return Status{Patched: enabled, Detail: detail}, nil
}
