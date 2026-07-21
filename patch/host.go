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
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"

	"github.com/beego/beego/v2/core/logs"
)

// Helpers every patcher needs: where the target user keeps their dot-files, and
// how to ask an agent's CLI to reload. Both are shared because every agent hides
// its configuration under the owner's home directory, and most need a restart
// before a freshly installed hook is live.

// agentCommandTimeout bounds a CLI call so a hung agent cannot hold the HTTP
// request open, and agentCommandOutputDelay bounds how long after the timeout
// the process gets to actually die.
const (
	agentCommandTimeout     = 20 * time.Second
	agentCommandOutputDelay = 2 * time.Second
)

// homeOf resolves the home directory of the account that owns an installation.
// Agents keep their hooks and config under it, so this is where every patch
// lands.
func homeOf(target Target) (string, error) {
	if target.Owner != "" {
		if account, err := user.Lookup(target.Owner); err == nil && account.HomeDir != "" {
			return account.HomeDir, nil
		}
	}
	return os.UserHomeDir()
}

// isCurrentUser reports whether owner is the account aiguard itself runs as.
// Environment overrides only describe that account, so patchers consult it
// before honouring one.
func isCurrentUser(owner string) bool {
	if owner == "" {
		return true
	}
	account, err := user.Current()
	if err != nil {
		return false
	}
	// Windows reports the current user as DOMAIN\name while scans report the
	// bare account name, so compare on the last segment.
	return strings.EqualFold(baseAccount(account.Username), baseAccount(owner))
}

func baseAccount(name string) string {
	if separator := strings.LastIndexAny(name, `\/`); separator >= 0 {
		return name[separator+1:]
	}
	return name
}

// requestAgentReload asks an agent's own CLI to restart, so it picks up a hook
// aiguard just installed or dropped.
//
// It returns immediately and does the work in the background, because a restart
// is not part of the patch: by the time this is called the hook is already on
// disk and the patch has succeeded, so an operator waiting on the HTTP response
// should not also be waiting on an agent's daemon supervisor - OpenClaw's
// gateway restart alone can take minutes. Whatever happens here, the worst case
// is that the operator restarts the agent by hand.
func requestAgentReload(name string, args ...string) {
	path, err := exec.LookPath(name)
	if err != nil {
		logs.Info("patch: %s is not on PATH, skipping %v (restart the agent to load the hook)", name, args)
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), agentCommandTimeout)
		defer cancel()

		// Deliberately no output capture: a restart leaves a daemon running that
		// inherits whatever it was given, so waiting for the pipes to close would
		// mean waiting for the daemon to exit.
		command := exec.CommandContext(ctx, path, args...)
		command.WaitDelay = agentCommandOutputDelay

		// The exit status is informational only - agent CLIs are inconsistent
		// about it, and OpenClaw's gateway restart reports failure on a restart
		// that worked.
		if err := command.Run(); err != nil {
			logs.Info("patch: %s %v returned %v - restart the agent by hand if the hook does not load", name, args, err)
		}
	}()
}

// jsonString renders a value as a JSON string literal, for substitution into a
// generated script where an unescaped Windows path would otherwise be read as
// an escape sequence.
func jsonString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(encoded)
}
