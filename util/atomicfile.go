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

package util

import (
	"os"
	"path/filepath"
)

// AtomicWriteFile writes data to path via a temp file in the same directory,
// then renames it into place, so a process killed mid-write leaves either the
// old file or the complete new one - never a truncated one.
//
// Replacing an existing file keeps that file's owner (see carryOwnerForward):
// the rename installs a new inode owned by the calling process, so without
// this an aiguard running as root would silently take over config files
// sitting in other users' home directories.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	// Stat first: once the rename lands there is nothing left to read the
	// previous owner from.
	existing, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	// Anything short of a completed rename leaves .tmp-* litter next to path.
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	// Ownership before mode: chown clears the setuid/setgid bits on most
	// Unixes, so the chmod has to be what runs last.
	if existing != nil {
		if err := carryOwnerForward(existing, tmpPath); err != nil {
			tmp.Close()
			return err
		}
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	renamed = true
	return nil
}
