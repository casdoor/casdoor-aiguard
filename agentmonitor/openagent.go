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
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/casdoor/casdoor-aiguard/conf"
	"github.com/casdoor/casdoor-aiguard/object"
)

// OpenAgent keeps all of its state in a SQLite database, so it exposes no hook
// directory and no config file to instrument. What it can be asked to do is
// write an append-only JSONL audit log next to that database - one
// self-contained event per line - which this monitor tails read-only, exactly
// the way the Codex monitor tails rollout files. Nothing on the agent is
// modified, so unpatching is just dropping the claim.
//
// Each audit line is complete on its own (tool call plus the guard verdict), so
// unlike the Codex monitor this one needs no turn or pending-call bookkeeping: a
// cursor is just a byte offset into a file.

const (
	openAgentMonitorStateFile = "openagent-audit-monitor.json"
	openAgentPollInterval     = time.Second
	maxOpenAgentAuditLine     = 8 * 1024 * 1024
	openAgentAgentID          = "openagent"
	openAgentAuditSubdir      = "audit"
)

type openAgentClaim struct {
	Path     string `json:"path"`
	Owner    string `json:"owner"`
	AuditDir string `json:"auditDir"`
}

type openAgentCursor struct {
	Path       string `json:"path"`
	Root       string `json:"root"`
	Offset     int64  `json:"offset"`
	SessionKey string `json:"sessionKey,omitempty"`
}

type openAgentSavedState struct {
	Claims  []openAgentClaim            `json:"claims"`
	Cursors map[string]*openAgentCursor `json:"cursors"`
}

type openAgentMonitorManager struct {
	mutex     sync.Mutex
	statePath string
	addRecord func(*object.Record)
	claims    map[string]openAgentClaim
	cursors   map[string]*openAgentCursor
	lastErr   map[string]error
	dirty     bool
	stop      chan struct{}
	done      chan struct{}
}

var openAgentMonitor = newOpenAgentMonitorManager("", object.AddRecord)

func newOpenAgentMonitorManager(statePath string, addRecord func(*object.Record)) *openAgentMonitorManager {
	if addRecord == nil {
		addRecord = object.AddRecord
	}
	return &openAgentMonitorManager{
		statePath: statePath,
		addRecord: addRecord,
		claims:    map[string]openAgentClaim{},
		cursors:   map[string]*openAgentCursor{},
		lastErr:   map[string]error{},
	}
}

func (m *openAgentMonitorManager) start() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.stop != nil {
		return nil
	}
	if m.statePath == "" {
		m.statePath = filepath.Join(conf.GetPatchStateDir(), openAgentMonitorStateFile)
	}
	if err := m.loadLocked(); err != nil {
		return err
	}
	m.startPollerLocked()
	return nil
}

func (m *openAgentMonitorManager) stopMonitor() {
	m.mutex.Lock()
	stop, done := m.stop, m.done
	m.stop, m.done = nil, nil
	m.mutex.Unlock()
	if stop != nil {
		close(stop)
		<-done
	}
}

// ResolveOpenAgentAuditDir locates the audit directory OpenAgent writes into. It
// sits next to the binary (the same directory strategy OpenAgent uses for its
// SQLite database), and OPENAGENT_AUDIT_DIR overrides it - honoured only for the
// current process user, since another user's environment is not ours to read.
func ResolveOpenAgentAuditDir(agentPath, owner string) (string, error) {
	owner = strings.TrimSpace(owner)
	if configured := strings.TrimSpace(os.Getenv("OPENAGENT_AUDIT_DIR")); configured != "" {
		current, _ := user.Current()
		if current != nil && strings.EqualFold(accountName(owner), accountName(current.Username)) {
			if !filepath.IsAbs(configured) {
				return "", errors.New("OPENAGENT_AUDIT_DIR must be an absolute path")
			}
			return filepath.Clean(configured), nil
		}
	}
	if agentPath == "" {
		return "", errors.New("agent path is required to locate the audit directory")
	}
	binary := agentPath
	if resolved, err := filepath.EvalSymlinks(agentPath); err == nil {
		binary = resolved
	}
	return filepath.Join(filepath.Dir(filepath.Clean(binary)), openAgentAuditSubdir), nil
}

func accountName(value string) string {
	return filepath.Base(strings.ReplaceAll(strings.TrimSpace(value), `\`, "/"))
}

func EnableOpenAgentMonitor(path, owner, auditDir string) error {
	return openAgentMonitor.enable(openAgentClaim{
		Path: filepath.Clean(path), Owner: strings.TrimSpace(owner), AuditDir: filepath.Clean(auditDir),
	})
}

func (m *openAgentMonitorManager) enable(claim openAgentClaim) error {
	if claim.Path == "" || claim.Owner == "" || !filepath.IsAbs(claim.AuditDir) {
		return errors.New("agent path, owner and an absolute audit directory are required")
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.statePath == "" {
		m.statePath = filepath.Join(conf.GetPatchStateDir(), openAgentMonitorStateFile)
	}
	key := openAgentClaimKey(claim.Path, claim.Owner)
	if _, exists := m.claims[key]; exists {
		return nil
	}
	if !m.hasClaimForRootLocked(claim.AuditDir) {
		if err := m.seedRootLocked(claim.AuditDir); err != nil {
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

func DisableOpenAgentMonitor(path, owner string) error {
	return openAgentMonitor.disable(path, owner)
}

func (m *openAgentMonitorManager) disable(path, owner string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	key := openAgentClaimKey(path, owner)
	previous, existed := m.claims[key]
	if !existed {
		return nil
	}
	delete(m.claims, key)
	m.dirty = true
	if err := m.saveLocked(); err != nil {
		m.claims[key] = previous
		return err
	}
	return nil
}

func OpenAgentMonitorStatus(path, owner string) (bool, string) {
	return openAgentMonitor.status(path, owner)
}

func (m *openAgentMonitorManager) status(path, owner string) (bool, string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	claim, ok := m.claims[openAgentClaimKey(path, owner)]
	if !ok {
		return false, "not patched"
	}
	root := claim.AuditDir
	files, exists, err := openAgentAuditFiles(root)
	if err == nil {
		err = m.lastErr[openAgentPathKey(root)]
	}
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return true, "permission denied: " + err.Error()
		}
		return true, "monitor error: " + err.Error()
	}
	if !exists {
		return true, "waiting for activity: audit directory not found"
	}
	if len(files) == 0 {
		return true, "waiting for activity: no audit files"
	}
	return true, fmt.Sprintf("active: monitoring %d audit file(s)", len(files))
}

func (m *openAgentMonitorManager) startPollerLocked() {
	if m.stop != nil {
		return
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	m.stop, m.done = stop, done
	go m.run(stop, done)
}

func (m *openAgentMonitorManager) run(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	m.poll()
	ticker := time.NewTicker(openAgentPollInterval)
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

func (m *openAgentMonitorManager) poll() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	roots := map[string]string{}
	for _, claim := range m.claims {
		roots[openAgentPathKey(claim.AuditDir)] = claim.AuditDir
	}
	for key, root := range roots {
		files, _, err := openAgentAuditFiles(root)
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

// seedRootLocked marks every audit file already present as read up to its
// current end, so enabling the monitor never replays history the operator did
// not ask for. Only lines appended after the claim are turned into records.
func (m *openAgentMonitorManager) seedRootLocked(root string) error {
	files, _, err := openAgentAuditFiles(root)
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

func (m *openAgentMonitorManager) cursorForFileLocked(root, path string, seedEOF bool) (*openAgentCursor, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	key := openAgentPathKey(path)
	cursor := m.cursors[key]
	if cursor == nil {
		cursor = &openAgentCursor{
			Path: path, Root: root, SessionKey: openAgentSessionKeyFromPath(path),
		}
		m.cursors[key] = cursor
		m.dirty = true
	}

	if seedEOF {
		cursor.Offset = info.Size()
		m.dirty = true
	} else if info.Size() < cursor.Offset {
		// The file was truncated or replaced; start over from the top.
		cursor.Offset = 0
		m.dirty = true
	}
	return cursor, nil
}

func (m *openAgentMonitorManager) consumeFileLocked(cursor *openAgentCursor) error {
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

	claim := m.claimForCursorLocked(cursor)
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
		if len(line) <= maxOpenAgentAuditLine {
			for _, record := range parseOpenAgentAuditLine(bytes.TrimSpace(line), cursor, claim) {
				m.addRecord(record)
			}
		}
	}
}

func (m *openAgentMonitorManager) claimForCursorLocked(cursor *openAgentCursor) *openAgentClaim {
	for _, claim := range m.claims {
		if openAgentPathKey(claim.AuditDir) == openAgentPathKey(cursor.Root) {
			return &claim
		}
	}
	return nil
}

func (m *openAgentMonitorManager) hasClaimForRootLocked(root string) bool {
	for _, claim := range m.claims {
		if openAgentPathKey(claim.AuditDir) == openAgentPathKey(root) {
			return true
		}
	}
	return false
}

func openAgentAuditFiles(root string) ([]string, bool, error) {
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.IsDir() {
		return nil, true, fmt.Errorf("OpenAgent audit root is not a directory: %s", root)
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
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, true, err
}

func openAgentSessionKeyFromPath(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func openAgentClaimKey(path, owner string) string {
	return strings.ToLower(filepath.Clean(path) + "\x00" + strings.TrimSpace(owner))
}

func openAgentPathKey(path string) string {
	key := filepath.Clean(path)
	if os.PathSeparator == '\\' {
		key = strings.ToLower(key)
	}
	return key
}

func (m *openAgentMonitorManager) loadLocked() error {
	data, err := os.ReadFile(m.statePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var saved openAgentSavedState
	if err := json.Unmarshal(data, &saved); err != nil {
		return fmt.Errorf("cannot parse OpenAgent audit monitor state: %w", err)
	}
	for _, claim := range saved.Claims {
		m.claims[openAgentClaimKey(claim.Path, claim.Owner)] = claim
	}
	if saved.Cursors != nil {
		m.cursors = saved.Cursors
	}
	repaired := false
	for key, cursor := range m.cursors {
		if cursor == nil {
			delete(m.cursors, key)
			repaired = true
		}
	}
	m.dirty = repaired
	return nil
}

func (m *openAgentMonitorManager) saveLocked() error {
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
	saved := openAgentSavedState{
		Claims:  make([]openAgentClaim, 0, len(m.claims)),
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
