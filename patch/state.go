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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/casdoor/casdoor-aiguard/conf"
)

// Patching an agent means editing files that belong to someone else - a hook
// directory here, a line of JSON in a config file there. Undoing that reliably
// needs more than "delete what we think we added": the config file may have had
// content of its own, and a half-finished patch must not leave the agent broken.
//
// So every patcher that uses this journal makes its edits through a ChangeSet,
// which copies each file aside before touching it and records what it did. The
// record (a manifest) is saved next to the backups, and Revert replays it
// backwards.

// changeKind distinguishes the two things a patch creates on disk, because they
// are undone differently: a file is restored or deleted, a directory is removed
// only when the patch created it and nothing else moved in.
type changeKind string

const (
	changeFile changeKind = "file"
	changeDir  changeKind = "dir"
)

// change is one filesystem modification, plus what it takes to undo it.
type change struct {
	Kind changeKind `json:"kind"`
	Path string     `json:"path"`
	// Backup is the copy of the file's pre-patch content, relative to the
	// manifest's backup directory. Empty means the path did not exist before
	// the patch, so undoing it means deleting it.
	Backup string `json:"backup,omitempty"`
	// Mode is the file's pre-patch permission bits, restored along with the
	// content. Zero when the path did not exist.
	Mode os.FileMode `json:"mode,omitempty"`
}

// manifest is the complete record of one patch, and the only thing Revert needs
// in order to undo it.
type manifest struct {
	AgentId   string    `json:"agentId"`
	Target    Target    `json:"target"`
	PatchedAt time.Time `json:"patchedAt"`
	// Changes are in the order they were applied; Revert walks them backwards.
	Changes []change `json:"changes"`
}

// stateMutex serializes patch and unpatch runs so two requests cannot interleave
// their edits to the same agent's files.
var stateMutex sync.Mutex

// ChangeSet is the only way a patcher is allowed to touch the filesystem. Each
// method backs up what it is about to overwrite and appends to the manifest, so
// the patcher can concentrate on what the agent needs rather than on how to put
// it back.
type ChangeSet struct {
	manifest  *manifest
	backupDir string
}

// MkdirAll creates dir and any missing parents, recording only the ones that did
// not already exist so Revert leaves pre-existing directories alone.
func (c *ChangeSet) MkdirAll(dir string) error {
	var created []string
	for current := filepath.Clean(dir); ; current = filepath.Dir(current) {
		if _, err := os.Stat(current); err == nil {
			break
		}
		created = append(created, current)
		if parent := filepath.Dir(current); parent == current {
			break
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// Record shallowest first, matching the order they were created, so the
	// reversed replay removes the deepest directory first.
	for i := len(created) - 1; i >= 0; i-- {
		c.manifest.Changes = append(c.manifest.Changes, change{Kind: changeDir, Path: created[i]})
	}
	return nil
}

// WriteFile writes data to path, first copying any existing content aside. perm
// applies only when the file is new; an existing file keeps its own mode.
func (c *ChangeSet) WriteFile(path string, data []byte, perm os.FileMode) error {
	record := change{Kind: changeFile, Path: path}

	existing, err := os.ReadFile(path)
	switch {
	case err == nil:
		info, statErr := os.Stat(path)
		if statErr != nil {
			return statErr
		}
		record.Mode = info.Mode().Perm()
		perm = record.Mode

		backup := fmt.Sprintf("%d-%s", len(c.manifest.Changes), filepath.Base(path))
		if err := os.MkdirAll(c.backupDir, 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(c.backupDir, backup), existing, 0o600); err != nil {
			return err
		}
		record.Backup = backup
	case os.IsNotExist(err):
		// Left with an empty Backup, which Revert reads as "delete it".
	default:
		return err
	}

	c.manifest.Changes = append(c.manifest.Changes, record)
	if err := os.WriteFile(path, data, perm); err != nil {
		return err
	}
	return nil
}

// chownCreated applies ownership only when this ChangeSet created path.
// Existing files are written in place, preserving their owner and ACL.
func (c *ChangeSet) chownCreated(path string, ownership fileOwnership) error {
	for index := len(c.manifest.Changes) - 1; index >= 0; index-- {
		item := c.manifest.Changes[index]
		if item.Path != path {
			continue
		}
		if item.Kind == changeDir || item.Kind == changeFile && item.Backup == "" {
			return applyFileOwnership(path, ownership)
		}
		return nil
	}
	return nil
}

func (c *ChangeSet) chmodCreated(path string, mode os.FileMode) error {
	for index := len(c.manifest.Changes) - 1; index >= 0; index-- {
		item := c.manifest.Changes[index]
		if item.Path == path && item.Kind == changeDir {
			return os.Chmod(path, mode)
		}
	}
	return nil
}

// ReadFile reads a file the patch is about to modify, reporting a missing file
// as empty content. Patchers use it to merge into an existing config rather than
// clobber it.
func (c *ChangeSet) ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}

// Apply runs a patcher's edits under the undo bookkeeping and saves the
// resulting manifest. If apply fails partway through, everything it managed to
// do is rolled back before the error is returned, so a failed patch never leaves
// the agent half-instrumented.
func Apply(target Target, apply func(*ChangeSet) error) error {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	// A previous patch's manifest would be orphaned by a second Apply, so undo
	// it first and patch from a known-clean state.
	if err := revertLocked(target); err != nil {
		return err
	}

	changes := &ChangeSet{
		manifest:  &manifest{AgentId: target.AgentId, Target: target, PatchedAt: time.Now()},
		backupDir: backupDir(target),
	}
	if err := apply(changes); err != nil {
		rollbackErr := rollback(changes.manifest, changes.backupDir)
		if rollbackErr != nil {
			recoveryErr := saveManifest(target, changes.manifest)
			if recoveryErr != nil {
				return fmt.Errorf("apply patch: %v; rollback failed: %v; keep recovery backup %s: %w", err, rollbackErr, changes.backupDir, recoveryErr)
			}
			return fmt.Errorf("apply patch: %v; rollback failed (recovery state retained): %w", err, rollbackErr)
		}
		_ = os.RemoveAll(changes.backupDir)
		return err
	}
	if err := saveManifest(target, changes.manifest); err != nil {
		rollbackErr := rollback(changes.manifest, changes.backupDir)
		_ = os.Remove(manifestPath(target))
		if rollbackErr != nil {
			return fmt.Errorf("save patch state: %v; rollback failed (recovery backup retained at %s): %w", err, changes.backupDir, rollbackErr)
		}
		_ = os.RemoveAll(changes.backupDir)
		return err
	}
	return nil
}

// edit applies a short-lived transaction to files that already have a durable
// Apply journal. Its backups exist only for this request: success discards
// them, while any later write or ownership failure restores the request's
// starting state without replacing the original journal used by Revert.
func edit(target Target, apply func(*ChangeSet) error) error {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	backups, err := os.MkdirTemp(stateDir(), ".edit-")
	if err != nil {
		return err
	}
	removeBackups := true
	defer func() {
		if removeBackups {
			_ = os.RemoveAll(backups)
		}
	}()

	changes := &ChangeSet{
		manifest:  &manifest{AgentId: target.AgentId, Target: target, PatchedAt: time.Now()},
		backupDir: backups,
	}
	if err := apply(changes); err != nil {
		if rollbackErr := rollback(changes.manifest, changes.backupDir); rollbackErr != nil {
			removeBackups = false
			return fmt.Errorf("edit files: %v; rollback failed (recovery backup retained at %s): %w", err, backups, rollbackErr)
		}
		return err
	}
	return nil
}

// Revert undoes the patch recorded for target. An unpatched target is not an
// error - there is simply nothing to undo.
func Revert(target Target) error {
	stateMutex.Lock()
	defer stateMutex.Unlock()
	return revertLocked(target)
}

func revertLocked(target Target) error {
	saved, err := loadManifest(target)
	if err != nil || saved == nil {
		return err
	}
	if err := rollback(saved, backupDir(target)); err != nil {
		return err
	}
	if err := os.Remove(manifestPath(target)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.RemoveAll(backupDir(target))
}

// IsApplied reports whether aiguard has a patch recorded for target. Patchers
// combine it with their own probe of the agent's files, since a user can always
// undo a patch by hand.
func IsApplied(target Target) bool {
	stateMutex.Lock()
	defer stateMutex.Unlock()
	saved, err := loadManifest(target)
	return err == nil && saved != nil
}

// rollback replays a manifest backwards, restoring each file and removing the
// directories the patch created. It keeps going after a failed step so one
// stubborn file cannot strand the rest of the changes.
func rollback(saved *manifest, backups string) error {
	var failure error
	for i := len(saved.Changes) - 1; i >= 0; i-- {
		item := saved.Changes[i]
		var err error
		switch item.Kind {
		case changeDir:
			// os.Remove refuses a non-empty directory, which is exactly what we
			// want: the patch created this directory, but anything that moved in
			// since is not ours to delete. Either way there is nothing to report.
			_ = os.Remove(item.Path)
		case changeFile:
			if item.Backup == "" {
				if err = os.Remove(item.Path); os.IsNotExist(err) {
					err = nil
				}
				break
			}
			var content []byte
			if content, err = os.ReadFile(filepath.Join(backups, item.Backup)); err == nil {
				// A managed file may originally have been read-only. Make it
				// writable long enough to restore its bytes, then put its exact
				// original mode back.
				if chmodErr := os.Chmod(item.Path, item.Mode|0o200); chmodErr != nil && !os.IsNotExist(chmodErr) {
					err = chmodErr
					break
				}
				err = os.WriteFile(item.Path, content, item.Mode|0o200)
				if err == nil {
					err = os.Chmod(item.Path, item.Mode)
				}
			}
		}
		if err != nil && failure == nil {
			failure = fmt.Errorf("failed to restore %s: %w", item.Path, err)
		}
	}
	return failure
}

func stateDir() string {
	return conf.GetPatchStateDir()
}

// targetKey identifies one installation. The path is hashed because it is a
// full filesystem path: too long, and full of separators, to be a file name.
func targetKey(target Target) string {
	sum := sha256.Sum256([]byte(target.Owner + "|" + target.Path))
	return target.AgentId + "-" + hex.EncodeToString(sum[:])[:16]
}

func manifestPath(target Target) string {
	return filepath.Join(stateDir(), targetKey(target)+".json")
}

func backupDir(target Target) string {
	return filepath.Join(stateDir(), targetKey(target))
}

// loadManifest returns the saved manifest, or nil when the target is unpatched.
func loadManifest(target Target) (*manifest, error) {
	data, err := os.ReadFile(manifestPath(target))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var saved manifest
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, err
	}
	return &saved, nil
}

func saveManifest(target Target, saved *manifest) error {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath(target), data, 0o600)
}
