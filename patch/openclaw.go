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
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/casdoor/casdoor-aiguard/conf"
)

// OpenClaw has a first-class extension point for this: a "managed hook" is a
// directory under ~/.openclaw/hooks/ holding a HOOK.md manifest and a handler
// module, which the gateway discovers at startup and runs on every agent event.
// Patching OpenClaw therefore means two edits - drop the hook directory in, and
// enable it in ~/.openclaw/openclaw.json - and unpatching means taking both back
// out. See https://docs.openclaw.ai/automation/hooks.

const (
	openclawHookName   = "aiguard-recorder"
	openclawStateDir   = ".openclaw"
	openclawConfigFile = "openclaw.json"
)

// The handler ships as a plain ESM .js file: OpenClaw imports it directly, and
// Node's module-syntax detection resolves it as ESM without the hook directory
// needing a package.json of its own.
//
//go:embed openclaw_hook.js
var openclawHookHandler string

//go:embed openclaw_hook.md
var openclawHookManifest string

func init() {
	register(openclawPatcher{})
}

type openclawPatcher struct{}

func (openclawPatcher) AgentId() string { return "openclaw" }

func (openclawPatcher) Supported() bool { return true }

func (p openclawPatcher) Patch(target Target) error {
	layout, err := p.layoutOf(target)
	if err != nil {
		return err
	}

	err = Apply(target, func(changes *ChangeSet) error {
		if err := changes.MkdirAll(layout.hookDir); err != nil {
			return err
		}
		if err := changes.WriteFile(filepath.Join(layout.hookDir, "HOOK.md"), []byte(openclawHookManifest), 0o644); err != nil {
			return err
		}
		handler := renderOpenclawHandler(target)
		if err := changes.WriteFile(filepath.Join(layout.hookDir, "handler.js"), []byte(handler), 0o644); err != nil {
			return err
		}
		return p.enableHook(changes, layout.configPath)
	})
	if err != nil {
		return err
	}

	// The gateway loads hooks once, at startup, so the hook only starts
	// reporting after a restart. Ask for one, but do not make the patch wait on
	// it or depend on it: the operator can always restart the gateway by hand.
	restartOpenclawGateway()
	return nil
}

func (p openclawPatcher) Unpatch(target Target) error {
	// Revert restores openclaw.json to its exact pre-patch bytes and deletes the
	// hook directory, so there is nothing OpenClaw-specific to undo by hand.
	if err := Revert(target); err != nil {
		return err
	}
	restartOpenclawGateway()
	return nil
}

func (p openclawPatcher) Status(target Target) (Status, error) {
	layout, err := p.layoutOf(target)
	if err != nil {
		return Status{}, err
	}

	// Probe the agent itself rather than trusting aiguard's manifest: a user can
	// always delete the hook directory or flip the config entry by hand, and the
	// table should show what is actually true.
	if _, err := os.Stat(filepath.Join(layout.hookDir, "handler.js")); err != nil {
		return Status{Detail: "not patched"}, nil
	}
	enabled, err := p.hookEnabled(layout.configPath)
	if err != nil {
		return Status{}, err
	}
	if !enabled {
		return Status{Detail: "hook installed but disabled in openclaw.json - unpatch and patch again"}, nil
	}
	if !IsApplied(target) {
		return Status{Patched: true, Detail: "patched outside aiguard: unpatch cannot restore the original config"}, nil
	}
	return Status{Patched: true}, nil
}

// openclawLayout is where one installation keeps the two things a patch touches.
type openclawLayout struct {
	hookDir    string
	configPath string
}

func (p openclawPatcher) layoutOf(target Target) (openclawLayout, error) {
	home, err := homeOf(target)
	if err != nil {
		return openclawLayout{}, err
	}
	// OPENCLAW_STATE_DIR relocates the whole state directory, so honour it -
	// but only for the current user, since another user's environment is not
	// ours to read.
	stateDir := filepath.Join(home, openclawStateDir)
	if relocated := os.Getenv("OPENCLAW_STATE_DIR"); relocated != "" && isCurrentUser(target.Owner) {
		stateDir = relocated
	}
	return openclawLayout{
		hookDir:    filepath.Join(stateDir, "hooks", openclawHookName),
		configPath: filepath.Join(stateDir, openclawConfigFile),
	}, nil
}

// renderOpenclawHandler bakes this aiguard's endpoint into the handler. The
// endpoint is a constant in the file rather than a config entry because the hook
// runs inside the gateway process, where aiguard controls nothing about the
// environment.
func renderOpenclawHandler(target Target) string {
	handler := strings.ReplaceAll(openclawHookHandler, "__AIGUARD_RECORDS_URL__", jsonString(conf.GetRecordsIngestUrl()))
	handler = strings.ReplaceAll(handler, "__AIGUARD_ENFORCE_URL__", jsonString(conf.GetEnforceUrl()))
	return strings.ReplaceAll(handler, "__AIGUARD_AGENT_PATH__", jsonString(target.Path))
}

// enableHook turns the hook on in openclaw.json without disturbing anything else
// in it: the file is round-tripped as a generic tree, so settings aiguard knows
// nothing about survive untouched.
func (p openclawPatcher) enableHook(changes *ChangeSet, configPath string) error {
	config, err := readJSONObject(changes, configPath)
	if err != nil {
		return err
	}

	hooks := childObject(config, "hooks")
	internal := childObject(hooks, "internal")
	internal["enabled"] = true
	entries := childObject(internal, "entries")
	entry := childObject(entries, openclawHookName)
	entry["enabled"] = true

	updated, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	// The config holds provider API keys, so keep it owner-only if we create it.
	return changes.WriteFile(configPath, append(updated, '\n'), 0o600)
}

func (p openclawPatcher) hookEnabled(configPath string) (bool, error) {
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return false, fmt.Errorf("cannot parse %s: %w", configPath, err)
	}

	internal, _ := lookupObject(config, "hooks", "internal")
	if enabled, ok := internal["enabled"].(bool); !ok || !enabled {
		return false, nil
	}
	entry, ok := lookupObject(internal, "entries", openclawHookName)
	if !ok {
		return false, nil
	}
	enabled, ok := entry["enabled"].(bool)
	return ok && enabled, nil
}

// childObject returns parent[key] as an object, creating it when it is missing
// or holds something that is not an object.
func childObject(parent map[string]any, key string) map[string]any {
	if child, ok := parent[key].(map[string]any); ok {
		return child
	}
	child := map[string]any{}
	parent[key] = child
	return child
}

// lookupObject walks a chain of object keys, reporting whether the whole path
// resolved to an object.
func lookupObject(root map[string]any, keys ...string) (map[string]any, bool) {
	current := root
	for _, key := range keys {
		child, ok := current[key].(map[string]any)
		if !ok {
			return map[string]any{}, false
		}
		current = child
	}
	return current, true
}

// restartOpenclawGateway asks the gateway to reload, since it only discovers
// hooks at startup. It returns without waiting - see requestAgentReload.
func restartOpenclawGateway() {
	requestAgentReload("openclaw", "gateway", "restart")
}
