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

//go:build !windows

package util

import (
	"os"
	"syscall"
)

// carryOwnerForward chowns replacement to the uid/gid of existing, so a
// temp-file + rename write leaves the destination owned by whoever owned it
// before. aiguard may run as root while editing a config file in another
// user's home directory (every patch.Target carries an Owner); without this,
// switching that user's Claude Code to a relay would turn their
// ~/.claude/settings.json into a root-owned file they can no longer write.
//
// No-op when the temp file already has the right owner, so an unprivileged
// single-user install never issues a chown at all.
func carryOwnerForward(existing os.FileInfo, replacement string) error {
	stat, ok := existing.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	uid, gid := int(stat.Uid), int(stat.Gid)

	info, err := os.Stat(replacement)
	if err != nil {
		return err
	}
	if current, ok := info.Sys().(*syscall.Stat_t); ok &&
		int(current.Uid) == uid && int(current.Gid) == gid {
		return nil
	}
	return os.Chown(replacement, uid, gid)
}
