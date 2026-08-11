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
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

func ownerOf(t *testing.T, path string) (int, int) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("no syscall.Stat_t on this platform")
	}
	return int(stat.Uid), int(stat.Gid)
}

// secondaryGid returns a group the current user belongs to that is not the
// gid path already has, or skips. Changing a file's group to one of your own
// groups needs no privilege, which is what lets the unprivileged test below
// exercise the same "the replacement must not take the original's ownership"
// path the root test exercises with uids.
func secondaryGid(t *testing.T, path string) int {
	t.Helper()
	_, currentGid := ownerOf(t, path)

	account, err := user.Current()
	if err != nil {
		t.Skip("cannot resolve current user:", err)
	}
	groups, err := account.GroupIds()
	if err != nil {
		t.Skip("cannot list the current user's groups:", err)
	}
	for _, group := range groups {
		gid, err := strconv.Atoi(group)
		if err != nil || gid == currentGid {
			continue
		}
		// Not every listed group is actually chgrp-able here; take the
		// first one the kernel accepts.
		if os.Chown(path, -1, gid) == nil {
			return gid
		}
	}
	t.Skip("this user has no second group to test ownership carry-forward with")
	return 0
}

// TestAtomicWriteFileKeepsExistingOwner is the unprivileged regression test
// for the ownership carry-forward. A rename installs a new inode whose group
// comes from the process (Linux) or the parent directory (BSD/macOS), not
// necessarily the replaced file's - the same mechanism that turns another
// user's config file root-owned when aiguard runs as root.
func TestAtomicWriteFileKeepsExistingOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Move it into a second group, so "preserved" means something stronger
	// than "happened to match the writer's default".
	_ = secondaryGid(t, path)
	wantUid, wantGid := ownerOf(t, path)

	if err := AtomicWriteFile(path, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	if uid, gid := ownerOf(t, path); uid != wantUid || gid != wantGid {
		t.Errorf("owner = %d:%d, want %d:%d - the rename did not carry the original's ownership forward", uid, gid, wantUid, wantGid)
	}
}

// TestAtomicWriteFilePreservesForeignOwnerAsRoot covers the uid half of the
// same guarantee: aiguard as root editing another user's config file must not
// leave it root-owned. Only root can chown a file away to another uid.
func TestAtomicWriteFilePreservesForeignOwnerAsRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to create a file owned by a different user")
	}

	const nobody = 65534
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, nobody, nobody); err != nil {
		t.Skipf("cannot chown to %d on this system: %v", nobody, err)
	}

	if err := AtomicWriteFile(path, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	uid, _ := ownerOf(t, path)
	if uid != nobody {
		t.Errorf("owner uid = %d, want %d - the rename took ownership away from the agent's user", uid, nobody)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Errorf("content = %q, want %q", data, "new")
	}
}

// TestAtomicWriteFileNewFileIsOwnedByTheWriter pins that carry-forward applies
// only to replacements: a file created from scratch has no previous owner, so
// it belongs to the writer and the caller must chown it if need be.
func TestAtomicWriteFileNewFileIsOwnedByTheWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "created.json")
	if err := AtomicWriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	uid, _ := ownerOf(t, path)
	if uid != os.Geteuid() {
		t.Errorf("owner uid = %d, want the writing process's %d", uid, os.Geteuid())
	}
}
