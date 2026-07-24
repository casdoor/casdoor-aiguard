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
	"fmt"
	"os"
	"path/filepath"
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
	targets    map[string]monitorTarget
	transcript *transcriptMonitor
}

var desktopMonitor = monitorManager{targets: map[string]monitorTarget{}}

// Start restores the installations the operator previously patched and begins
// watching their Cowork audit logs from the current end of each file.
func Start() error {
	desktopMonitor.mutex.Lock()
	defer desktopMonitor.mutex.Unlock()

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
		target.Path = filepath.Clean(target.Path)
		desktopMonitor.targets[targetKey(target.Path)] = target
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
	target := monitorTarget{Path: filepath.Clean(path), Owner: strings.TrimSpace(owner)}

	desktopMonitor.mutex.Lock()
	defer desktopMonitor.mutex.Unlock()

	key := targetKey(target.Path)
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
	desktopMonitor.mutex.Lock()
	defer desktopMonitor.mutex.Unlock()

	key := targetKey(path)
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
	desktopMonitor.mutex.Lock()
	defer desktopMonitor.mutex.Unlock()
	if _, ok := desktopMonitor.targets[targetKey(path)]; !ok {
		return false, "not patched"
	}
	if desktopMonitor.transcript == nil {
		return true, "Cowork transcript monitor enabled but inactive"
	}
	status := desktopMonitor.transcript.Status()
	if status.lastErr != nil {
		detail := "Cowork transcript monitor error: " + status.lastErr.Error()
		if len(status.existingRoots) != 0 {
			detail += "; discovered paths: " + strings.Join(status.existingRoots, ", ")
		}
		return true, detail
	}
	if len(status.existingRoots) == 0 {
		return true, "Cowork monitor enabled, but no audit directory was found; checked: " +
			strings.Join(status.configuredRoots, ", ")
	}
	if status.auditFileCount == 0 {
		return true, "Cowork monitor enabled, but no audit.jsonl was found; paths: " +
			strings.Join(status.existingRoots, ", ")
	}
	return true, fmt.Sprintf(
		"Cowork transcript monitor active: %d audit.jsonl file(s); last successful poll %s; paths: %s",
		status.auditFileCount,
		status.lastSuccessfulPoll.Format("2006-01-02T15:04:05.000Z07:00"),
		strings.Join(status.existingRoots, ", "),
	)
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
	saved := monitorState{Targets: make([]monitorTarget, 0, len(m.targets))}
	for _, target := range m.targets {
		saved.Targets = append(saved.Targets, target)
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func targetKey(path string) string {
	return strings.ToLower(filepath.Clean(path))
}

func monitorStatePath() string {
	return filepath.Join(conf.GetPatchStateDir(), monitorStateFile)
}
