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

package agentmonitor

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/casdoor/casdoor-aiguard/conf"
)

const (
	claudeDesktopAgentID = "claude-desktop"
	monitorStateFile     = "claude-desktop-monitor.json"
)

type monitorTarget struct {
	Path  string `json:"path"`
	Owner string `json:"owner,omitempty"`
}

type monitorState struct {
	Targets []monitorTarget `json:"targets"`
}

type monitorManager struct {
	mutex      sync.Mutex
	loaded     bool
	targets    map[string]monitorTarget
	transcript *transcriptMonitor
}

var desktopMonitor monitorManager

// Start restores the installations the operator previously patched and begins
// watching their Cowork audit logs from the current end of each file.
func Start() error {
	desktopMonitor.mutex.Lock()
	defer desktopMonitor.mutex.Unlock()

	if err := desktopMonitor.loadLocked(); err != nil {
		return err
	}
	if len(desktopMonitor.targets) != 0 && desktopMonitor.transcript == nil {
		desktopMonitor.transcript = newTranscriptMonitor(desktopMonitor.targets)
	}
	return nil
}

func Stop() {
	desktopMonitor.mutex.Lock()
	transcript := desktopMonitor.transcript
	desktopMonitor.transcript = nil
	desktopMonitor.mutex.Unlock()
	if transcript != nil {
		transcript.Stop()
	}
}

// Enable persists one installation and starts monitoring immediately. Cowork
// does not need to be running; a session directory created later is picked up
// by the transcript monitor.
func Enable(path, owner string) error {
	target, err := normalizeTarget(path, owner)
	if err != nil {
		return err
	}

	desktopMonitor.mutex.Lock()
	defer desktopMonitor.mutex.Unlock()
	if err := desktopMonitor.loadLocked(); err != nil {
		return err
	}

	key := targetKey(target)
	previous, existed := desktopMonitor.targets[key]
	desktopMonitor.targets[key] = target
	if err := desktopMonitor.saveLocked(); err != nil {
		if existed {
			desktopMonitor.targets[key] = previous
		} else {
			delete(desktopMonitor.targets, key)
		}
		return err
	}

	if desktopMonitor.transcript == nil {
		desktopMonitor.transcript = newTranscriptMonitor(desktopMonitor.targets)
	} else {
		desktopMonitor.transcript.SetTargets(desktopMonitor.targets)
	}
	return nil
}

func Disable(path string) error {
	target, err := normalizeTarget(path, "")
	if err != nil {
		return err
	}

	desktopMonitor.mutex.Lock()
	defer desktopMonitor.mutex.Unlock()
	if err := desktopMonitor.loadLocked(); err != nil {
		return err
	}

	key := targetKey(target)
	previous, existed := desktopMonitor.targets[key]
	if !existed {
		return nil
	}
	delete(desktopMonitor.targets, key)
	if err := desktopMonitor.saveLocked(); err != nil {
		desktopMonitor.targets[key] = previous
		return err
	}

	if desktopMonitor.transcript == nil {
		return nil
	}
	if len(desktopMonitor.targets) == 0 {
		desktopMonitor.transcript.Stop()
		desktopMonitor.transcript = nil
	} else {
		desktopMonitor.transcript.SetTargets(desktopMonitor.targets)
	}
	return nil
}

func Status(path string) (bool, string) {
	target, err := normalizeTarget(path, "")
	if err != nil {
		return false, err.Error()
	}

	desktopMonitor.mutex.Lock()
	defer desktopMonitor.mutex.Unlock()
	if err := desktopMonitor.loadLocked(); err != nil {
		return false, err.Error()
	}
	if _, ok := desktopMonitor.targets[targetKey(target)]; !ok {
		return false, "not patched"
	}
	if desktopMonitor.transcript == nil {
		return true, "Cowork transcript monitor enabled but inactive"
	}
	if err := desktopMonitor.transcript.Err(); err != nil {
		return true, "Cowork transcript monitor error: " + err.Error()
	}
	return true, "Cowork transcript monitor active"
}

func (m *monitorManager) loadLocked() error {
	if m.loaded {
		return nil
	}
	m.loaded = true
	m.targets = map[string]monitorTarget{}

	data, err := os.ReadFile(monitorStatePath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var saved monitorState
	if err := json.Unmarshal(data, &saved); err != nil {
		return fmt.Errorf("cannot parse Claude Desktop monitor state: %w", err)
	}
	for _, target := range saved.Targets {
		normalized, err := normalizeTarget(target.Path, target.Owner)
		if err != nil {
			return fmt.Errorf("invalid Claude Desktop monitor target: %w", err)
		}
		m.targets[targetKey(normalized)] = normalized
	}
	return nil
}

func (m *monitorManager) saveLocked() error {
	path := monitorStatePath()
	if len(m.targets) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	keys := make([]string, 0, len(m.targets))
	for key := range m.targets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	saved := monitorState{Targets: make([]monitorTarget, 0, len(keys))}
	for _, key := range keys {
		saved.Targets = append(saved.Targets, m.targets[key])
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func normalizeTarget(path, owner string) (monitorTarget, error) {
	if strings.TrimSpace(path) == "" {
		return monitorTarget{}, errors.New("target path is required")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return monitorTarget{}, err
	}
	return monitorTarget{Path: filepath.Clean(path), Owner: strings.TrimSpace(owner)}, nil
}

func targetKey(target monitorTarget) string {
	return strings.ToLower(filepath.Clean(target.Path))
}

func monitorStatePath() string {
	return filepath.Join(conf.GetPatchStateDir(), monitorStateFile)
}
