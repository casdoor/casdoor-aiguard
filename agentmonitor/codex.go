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

package agentmonitor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/casdoor/casdoor-aiguard/conf"
	"github.com/casdoor/casdoor-aiguard/object"
)

const (
	codexMonitorStateFile = "codex-rollout-monitor.json"
	codexPollInterval     = time.Second
	maxCodexRolloutLine   = 8 * 1024 * 1024
)

type codexClaim struct {
	AgentID   string `json:"agentId"`
	Path      string `json:"path"`
	Owner     string `json:"owner"`
	CodexHome string `json:"codexHome"`
}

type codexPendingCall struct {
	Name      string    `json:"name"`
	StartedAt time.Time `json:"startedAt"`
	TurnID    string    `json:"turnId,omitempty"`
	Object    string    `json:"object,omitempty"`
}

type codexCursor struct {
	Path       string                      `json:"path"`
	Root       string                      `json:"root"`
	Offset     int64                       `json:"offset"`
	SessionKey string                      `json:"sessionKey,omitempty"`
	AgentID    string                      `json:"agentId,omitempty"`
	Model      string                      `json:"model,omitempty"`
	TurnID     string                      `json:"turnId,omitempty"`
	Pending    map[string]codexPendingCall `json:"pending,omitempty"`
}

type codexSavedState struct {
	Claims  []codexClaim            `json:"claims"`
	Cursors map[string]*codexCursor `json:"cursors"`
}

type codexMonitorManager struct {
	mutex     sync.Mutex
	statePath string
	addRecord func(*object.Record)
	claims    map[string]codexClaim
	cursors   map[string]*codexCursor
	lastErr   map[string]error
	dirty     bool
	stop      chan struct{}
	done      chan struct{}
}

var codexMonitor = newCodexMonitorManager("", object.AddRecord)

func newCodexMonitorManager(statePath string, addRecord func(*object.Record)) *codexMonitorManager {
	if addRecord == nil {
		addRecord = object.AddRecord
	}
	return &codexMonitorManager{
		statePath: statePath,
		addRecord: addRecord,
		claims:    map[string]codexClaim{},
		cursors:   map[string]*codexCursor{},
		lastErr:   map[string]error{},
	}
}

func (m *codexMonitorManager) start() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.stop != nil {
		return nil
	}
	if m.statePath == "" {
		m.statePath = filepath.Join(conf.GetPatchStateDir(), codexMonitorStateFile)
	}
	if err := m.loadLocked(); err != nil {
		return err
	}
	m.startPollerLocked()
	return nil
}

func (m *codexMonitorManager) stopMonitor() {
	m.mutex.Lock()
	stop, done := m.stop, m.done
	m.stop, m.done = nil, nil
	m.mutex.Unlock()
	if stop != nil {
		close(stop)
		<-done
	}
}

func EnableCodexMonitor(agentID, path, owner, codexHome string) error {
	return codexMonitor.enable(codexClaim{
		AgentID: agentID, Path: filepath.Clean(path), Owner: strings.TrimSpace(owner),
		CodexHome: filepath.Clean(codexHome),
	})
}

func (m *codexMonitorManager) enable(claim codexClaim) error {
	if claim.AgentID != "codex" && claim.AgentID != "codex-cli" {
		return fmt.Errorf("unsupported Codex rollout agent %q", claim.AgentID)
	}
	if claim.Path == "" || claim.Owner == "" || !filepath.IsAbs(claim.CodexHome) {
		return errors.New("agent path, owner and an absolute CODEX_HOME are required")
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.statePath == "" {
		m.statePath = filepath.Join(conf.GetPatchStateDir(), codexMonitorStateFile)
	}
	key := codexClaimKey(claim.AgentID, claim.Path, claim.Owner)
	if _, exists := m.claims[key]; exists {
		return nil
	}
	root := codexSessionsRoot(claim.CodexHome)
	if !m.hasClaimForRootLocked(root) {
		if err := m.seedRootLocked(root); err != nil {
			return err
		}
	}
	m.claims[key] = claim
	m.dirty = true
	if err := m.saveLocked(); err != nil {
		delete(m.claims, key)
		return err
	}
	return nil
}

func DisableCodexMonitor(agentID, path, owner string) error {
	return codexMonitor.disable(agentID, path, owner)
}

func (m *codexMonitorManager) disable(agentID, path, owner string) error {
	m.mutex.Lock()
	key := codexClaimKey(agentID, path, owner)
	previous, existed := m.claims[key]
	if !existed {
		m.mutex.Unlock()
		return nil
	}
	delete(m.claims, key)
	m.dirty = true
	if err := m.saveLocked(); err != nil {
		m.claims[key] = previous
		m.mutex.Unlock()
		return err
	}
	m.mutex.Unlock()
	return nil
}

func CodexMonitorStatus(agentID, path, owner string) (bool, string) {
	return codexMonitor.status(agentID, path, owner)
}

func (m *codexMonitorManager) status(agentID, path, owner string) (bool, string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	claim, ok := m.claims[codexClaimKey(agentID, path, owner)]
	if !ok {
		return false, "not patched"
	}
	root := codexSessionsRoot(claim.CodexHome)
	files, exists, err := codexRolloutFiles(root)
	if err == nil {
		err = m.lastErr[codexPathKey(root)]
	}
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return true, "permission denied: " + err.Error()
		}
		return true, "monitor error: " + err.Error()
	}
	if !exists {
		return true, "waiting for activity: sessions directory not found"
	}
	if len(files) == 0 {
		return true, "waiting for activity: no rollout files"
	}
	matchingFiles, knownFiles, unsupportedFiles := 0, 0, 0
	for _, cursor := range m.cursors {
		if codexPathKey(cursor.Root) != codexPathKey(root) {
			continue
		}
		if cursor.AgentID == "" {
			unsupportedFiles++
		} else {
			knownFiles++
			if cursor.AgentID == claim.AgentID {
				matchingFiles++
			}
		}
	}
	if knownFiles == 0 && unsupportedFiles > 0 {
		return true, fmt.Sprintf("unsupported source: %d rollout file(s) did not identify as ChatGPT Desktop Codex or Codex CLI", unsupportedFiles)
	}
	if matchingFiles == 0 {
		return true, "waiting for activity: no matching source"
	}
	return true, fmt.Sprintf("active: monitoring %d rollout file(s)", matchingFiles)
}

func (m *codexMonitorManager) startPollerLocked() {
	if m.stop != nil {
		return
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	m.stop, m.done = stop, done
	go m.run(stop, done)
}

func (m *codexMonitorManager) run(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	m.poll()
	ticker := time.NewTicker(codexPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.poll()
		case <-stop:
			return
		}
	}
}

func (m *codexMonitorManager) poll() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	roots := map[string]string{}
	for _, claim := range m.claims {
		root := codexSessionsRoot(claim.CodexHome)
		roots[codexPathKey(root)] = root
	}
	for key, root := range roots {
		files, _, err := codexRolloutFiles(root)
		if err != nil {
			m.lastErr[key] = err
			continue
		}
		delete(m.lastErr, key)
		for _, path := range files {
			cursor, err := m.cursorForFileLocked(root, path, false)
			if err != nil {
				m.lastErr[key] = err
				continue
			}
			if err := m.consumeFileLocked(cursor); err != nil {
				m.lastErr[key] = err
			}
		}
	}
	if m.dirty {
		if err := m.saveLocked(); err != nil {
			for key := range roots {
				m.lastErr[key] = err
			}
		}
	}
}

func (m *codexMonitorManager) seedRootLocked(root string) error {
	files, _, err := codexRolloutFiles(root)
	if err != nil {
		return err
	}
	for _, path := range files {
		if _, err := m.cursorForFileLocked(root, path, true); err != nil {
			return err
		}
	}
	return nil
}

func (m *codexMonitorManager) cursorForFileLocked(root, path string, seedEOF bool) (*codexCursor, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	key := codexPathKey(path)
	cursor := m.cursors[key]
	if cursor == nil {
		meta, err := codexFileHeader(path)
		if err != nil {
			return nil, err
		}
		cursor = &codexCursor{
			Path: path, Root: root, SessionKey: meta.SessionKey, AgentID: meta.AgentID,
			Pending: map[string]codexPendingCall{},
		}
		m.cursors[key] = cursor
		m.dirty = true
	}

	if seedEOF {
		cursor.Offset = info.Size()
		resetCodexCursorTurn(cursor)
		m.dirty = true
	} else if info.Size() < cursor.Offset {
		cursor.Offset = 0
		cursor.SessionKey = ""
		cursor.AgentID = ""
		resetCodexCursorTurn(cursor)
		m.dirty = true
	}
	if cursor.AgentID == "" && cursor.Offset == 0 && info.Size() > 0 {
		meta, err := codexFileHeader(path)
		if err != nil {
			return nil, err
		}
		cursor.SessionKey = meta.SessionKey
		cursor.AgentID = meta.AgentID
	}
	return cursor, nil
}

func resetCodexCursorTurn(cursor *codexCursor) {
	cursor.Model = ""
	cursor.TurnID = ""
	cursor.Pending = map[string]codexPendingCall{}
}

func (m *codexMonitorManager) consumeFileLocked(cursor *codexCursor) error {
	info, err := os.Stat(cursor.Path)
	if err != nil {
		return err
	}
	if info.Size() == cursor.Offset {
		return nil
	}

	file, err := os.Open(cursor.Path)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Seek(cursor.Offset, io.SeekStart); err != nil {
		return err
	}

	reader := bufio.NewReader(file)
	for {
		line, readErr := reader.ReadBytes('\n')
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}

		cursor.Offset += int64(len(line))
		m.dirty = true
		if len(line) <= maxCodexRolloutLine {
			claim := m.claimForCursorLocked(cursor)
			for _, record := range parseCodexRolloutLine(bytes.TrimSpace(line), cursor, claim) {
				m.addRecord(record)
			}
		}
	}
}

func (m *codexMonitorManager) claimForCursorLocked(cursor *codexCursor) *codexClaim {
	for _, claim := range m.claims {
		if claim.AgentID == cursor.AgentID &&
			codexPathKey(codexSessionsRoot(claim.CodexHome)) == codexPathKey(cursor.Root) {
			return &claim
		}
	}
	return nil
}

func (m *codexMonitorManager) hasClaimForRootLocked(root string) bool {
	for _, claim := range m.claims {
		if codexPathKey(codexSessionsRoot(claim.CodexHome)) == codexPathKey(root) {
			return true
		}
	}
	return false
}

func codexRolloutFiles(root string) ([]string, bool, error) {
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.IsDir() {
		return nil, true, fmt.Errorf("Codex sessions root is not a directory: %s", root)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, true, err
	}

	var files []string
	err = filepath.WalkDir(canonicalRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "rollout-") ||
			!strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, true, err
}

type codexHeaderMeta struct {
	SessionKey string
	AgentID    string
}

func codexFileHeader(path string) (codexHeaderMeta, error) {
	file, err := os.Open(path)
	if err != nil {
		return codexHeaderMeta{}, err
	}
	defer file.Close()
	line, err := bufio.NewReader(file).ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return codexHeaderMeta{}, err
	}
	return parseCodexHeader(bytes.TrimSpace(line)), nil
}

func codexSessionsRoot(codexHome string) string {
	return filepath.Join(filepath.Clean(codexHome), "sessions")
}

func codexClaimKey(agentID, path, owner string) string {
	return strings.ToLower(agentID + "\x00" + filepath.Clean(path) + "\x00" + strings.TrimSpace(owner))
}

func codexPathKey(path string) string {
	key := filepath.Clean(path)
	if os.PathSeparator == '\\' {
		key = strings.ToLower(key)
	}
	return key
}

func (m *codexMonitorManager) loadLocked() error {
	data, err := os.ReadFile(m.statePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var saved codexSavedState
	if err := json.Unmarshal(data, &saved); err != nil {
		return fmt.Errorf("cannot parse Codex rollout monitor state: %w", err)
	}
	for _, claim := range saved.Claims {
		m.claims[codexClaimKey(claim.AgentID, claim.Path, claim.Owner)] = claim
	}
	if saved.Cursors != nil {
		m.cursors = saved.Cursors
	}
	repaired := false
	for key, cursor := range m.cursors {
		if cursor == nil {
			delete(m.cursors, key)
			repaired = true
			continue
		}
		if cursor.Pending == nil {
			cursor.Pending = map[string]codexPendingCall{}
		}
		if cursor.AgentID == "" && cursor.Path != "" {
			if meta, err := codexFileHeader(cursor.Path); err == nil && meta.AgentID != "" {
				cursor.AgentID = meta.AgentID
				if cursor.SessionKey == "" {
					cursor.SessionKey = meta.SessionKey
				}
				repaired = true
			}
		}
	}
	m.dirty = repaired
	return nil
}

func (m *codexMonitorManager) saveLocked() error {
	if m.statePath == "" {
		return nil
	}
	if len(m.claims) == 0 && len(m.cursors) == 0 {
		if err := os.Remove(m.statePath); err != nil && !os.IsNotExist(err) {
			return err
		}
		m.dirty = false
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(m.statePath), 0o700); err != nil {
		return err
	}
	saved := codexSavedState{
		Claims:  make([]codexClaim, 0, len(m.claims)),
		Cursors: m.cursors,
	}
	for _, claim := range m.claims {
		saved.Claims = append(saved.Claims, claim)
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(m.statePath, append(data, '\n'), 0o600); err != nil {
		return err
	}
	m.dirty = false
	return nil
}
