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
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/beego/beego/v2/core/logs"
	"github.com/casdoor/casdoor-aiguard/auditutil"
	"github.com/casdoor/casdoor-aiguard/object"
)

const (
	transcriptPollInterval = time.Second
	maxTranscriptLine      = 8 * 1024 * 1024
)

type transcriptRoot struct {
	path   string
	target monitorTarget
}

type pendingToolCall struct {
	target    monitorTarget
	started   time.Time
	session   string
	id        string
	name      string
	model     string
	mcpServer string
	mcpTool   string
}

func (call pendingToolCall) record(when time.Time, outcome string) *object.Record {
	duration := when.Sub(call.started).Milliseconds()
	if duration < 0 {
		duration = 0
	}
	eventType := "tool"
	if call.mcpServer != "" {
		eventType = "mcp"
	}
	return &object.Record{
		CreatedTime: when.Format("2006-01-02T15:04:05.000Z07:00"),
		Agent:       claudeDesktopAgentID,
		AgentPath:   call.target.Path,
		User:        call.target.Owner,
		EventType:   eventType,
		Action:      "call",
		Outcome:     outcome,
		SessionKey:  call.session,
		ToolUseId:   call.id,
		ToolName:    call.name,
		McpServer:   call.mcpServer,
		McpTool:     call.mcpTool,
		Model:       call.model,
		DurationMs:  duration,
	}
}

type transcriptMonitor struct {
	mutex              sync.Mutex
	roots              map[string]transcriptRoot
	offsets            map[string]int64
	pending            map[string]pendingToolCall
	metadata           coworkMetadataCache
	stop               chan struct{}
	done               chan struct{}
	existingRoots      []string
	auditFileCount     int
	lastSuccessfulPoll time.Time
	lastErr            error
}

type transcriptStatus struct {
	configuredRoots    []string
	existingRoots      []string
	auditFileCount     int
	lastSuccessfulPoll time.Time
	lastErr            error
}

func newTranscriptMonitor(targets map[string]monitorTarget) *transcriptMonitor {
	monitor := &transcriptMonitor{
		roots:    map[string]transcriptRoot{},
		offsets:  map[string]int64{},
		pending:  map[string]pendingToolCall{},
		metadata: coworkMetadataCache{entries: map[string]coworkMetadataCacheEntry{}},
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	monitor.SetTargets(targets)
	monitor.poll()
	go monitor.run()
	return monitor
}

// SetTargets switches the set of Windows profiles whose Cowork audit logs are
// watched. Existing audit files under a newly enabled profile are seeded at
// EOF, so Patch never imports earlier sessions.
func (m *transcriptMonitor) SetTargets(targets map[string]monitorTarget) {
	currentHome, homeErr := os.UserHomeDir()
	next := map[string]transcriptRoot{}
	for _, target := range targets {
		profile := currentHome
		cleanPath := filepath.Clean(target.Path)
		marker := string(os.PathSeparator) + filepath.Join("AppData", "Local", "AnthropicClaude")
		if index := strings.Index(strings.ToLower(cleanPath), strings.ToLower(marker)); index >= 0 {
			profile = cleanPath[:index]
		}
		if profile == "" {
			continue
		}

		roaming := filepath.Join(profile, "AppData", "Roaming")
		local := filepath.Join(profile, "AppData", "Local")
		if homeErr == nil && strings.EqualFold(filepath.Clean(profile), filepath.Clean(currentHome)) {
			if configured := os.Getenv("APPDATA"); configured != "" {
				roaming = configured
			}
			if configured := os.Getenv("LOCALAPPDATA"); configured != "" {
				local = configured
			}
		}
		for _, root := range []string{
			filepath.Join(roaming, "Claude", "local-agent-mode-sessions"),
			filepath.Join(local, "Claude-3p", "local-agent-mode-sessions"),
			filepath.Join(local, "Packages", "Claude_pzs8sxrjxfjjc", "LocalCache", "Roaming", "Claude", "local-agent-mode-sessions"),
		} {
			root = filepath.Clean(root)
			next[strings.ToLower(root)] = transcriptRoot{path: root, target: target}
		}
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	if homeErr != nil {
		m.lastErr = homeErr
	}
	for key, root := range next {
		if _, exists := m.roots[key]; exists {
			continue
		}
		files, _, err := auditFiles(root.path)
		if err != nil {
			m.lastErr = err
			continue
		}
		for _, path := range files {
			if info, err := os.Stat(path); err == nil {
				m.offsets[strings.ToLower(filepath.Clean(path))] = info.Size()
			}
		}
	}
	m.roots = next
}

func (m *transcriptMonitor) Stop() {
	close(m.stop)
	<-m.done
}

func (m *transcriptMonitor) Status() transcriptStatus {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	status := transcriptStatus{
		existingRoots:      append([]string(nil), m.existingRoots...),
		auditFileCount:     m.auditFileCount,
		lastSuccessfulPoll: m.lastSuccessfulPoll,
		lastErr:            m.lastErr,
	}
	for _, root := range m.roots {
		status.configuredRoots = append(status.configuredRoots, root.path)
	}
	sort.Strings(status.configuredRoots)
	return status
}

func (m *transcriptMonitor) run() {
	defer close(m.done)
	ticker := time.NewTicker(transcriptPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.poll()
		case <-m.stop:
			return
		}
	}
}

func (m *transcriptMonitor) poll() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.lastErr = nil
	m.existingRoots = nil
	m.auditFileCount = 0
	for _, root := range m.roots {
		files, exists, err := auditFiles(root.path)
		if exists {
			m.existingRoots = append(m.existingRoots, root.path)
		}
		if err != nil {
			m.lastErr = err
			continue
		}
		m.auditFileCount += len(files)
		for _, path := range files {
			metadata, changed, metadataErr := m.metadata.load(path)
			if metadataErr != nil {
				logs.Warn("failed to read optional Cowork session metadata: %v", metadataErr)
			}
			sessionKey := metadata.sessionKey(path)
			if changed {
				object.SetSessionTitle(sessionKey, metadata.Title)
			}
			if err := m.consumeFile(path, root.target, sessionKey); err != nil {
				m.lastErr = err
			}
		}
	}
	sort.Strings(m.existingRoots)
	if m.lastErr == nil {
		m.lastSuccessfulPoll = time.Now()
	}
}

func auditFiles(root string) ([]string, bool, error) {
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.IsDir() {
		return nil, true, errors.New("Cowork audit root is not a directory: " + root)
	}

	var result []string
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if strings.EqualFold(entry.Name(), ".claude") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(entry.Name(), "audit.jsonl") {
			result = append(result, path)
		}
		return nil
	})
	return result, true, err
}

func (m *transcriptMonitor) consumeFile(path string, target monitorTarget, sessionKey string) error {
	key := strings.ToLower(filepath.Clean(path))
	offset := m.offsets[key]

	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() < offset {
		offset = 0
		m.offsets[key] = 0
	}
	if info.Size() == offset {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return err
	}

	reader := bufio.NewReaderSize(file, 64*1024)
	var line []byte
	var consumed int64
	oversized := false
	for {
		fragment, readErr := reader.ReadSlice('\n')
		consumed += int64(len(fragment))
		if !oversized {
			if len(line)+len(fragment) <= maxTranscriptLine {
				line = append(line, fragment...)
			} else {
				line = nil
				oversized = true
			}
		}

		if errors.Is(readErr, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}

		offset += consumed
		m.offsets[key] = offset
		if !oversized {
			m.consumeLine(target, sessionKey, bytes.TrimSpace(line))
		}
		line = line[:0]
		consumed = 0
		oversized = false
	}
}

func (m *transcriptMonitor) consumeLine(target monitorTarget, sessionKey string, line []byte) {
	if len(line) == 0 {
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	var entry map[string]any
	if decoder.Decode(&entry) != nil {
		return
	}

	message, _ := entry["message"].(map[string]any)
	content := entry["content"]
	if nested, exists := message["content"]; exists {
		content = nested
	}
	var blocks []map[string]any
	contentLength := 0
	switch value := content.(type) {
	case string:
		contentLength = utf8.RuneCountInString(value)
	case []any:
		for _, item := range value {
			switch item := item.(type) {
			case string:
				contentLength += utf8.RuneCountInString(item)
			case map[string]any:
				block := item
				blocks = append(blocks, block)
				if transcriptField(block, "type") == "text" {
					contentLength += utf8.RuneCountInString(transcriptField(block, "text"))
				}
			}
		}
	case map[string]any:
		blocks = append(blocks, value)
		if transcriptField(value, "type") == "text" {
			contentLength = utf8.RuneCountInString(transcriptField(value, "text"))
		}
	}
	model := transcriptField(message, "model")
	if model == "" {
		model = transcriptField(entry, "model")
	}
	when := time.Now()
	if timestamp := transcriptField(entry, "timestamp", "_audit_timestamp"); timestamp != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, timestamp); err == nil {
			when = parsed
		}
	}

	role := transcriptField(message, "role")
	if role == "" {
		role = transcriptField(entry, "role", "type")
	}
	if contentLength > 0 && (role == "user" || role == "assistant") {
		eventType, action, outcome := "prompt", "submitted", "attempted"
		if role == "assistant" {
			eventType, action, outcome = "response", "completed", "success"
		}
		payload, _ := json.Marshal(map[string]int{"contentLength": contentLength})
		object.AddRecord(&object.Record{
			CreatedTime: when.Format("2006-01-02T15:04:05.000Z07:00"),
			Agent:       claudeDesktopAgentID,
			AgentPath:   target.Path,
			User:        target.Owner,
			EventType:   eventType,
			Action:      action,
			Outcome:     outcome,
			SessionKey:  sessionKey,
			Model:       model,
			Object:      string(payload),
		})
	}

	for _, block := range blocks {
		switch transcriptField(block, "type") {
		case "tool_use":
			name := transcriptField(block, "name")
			if name == "" {
				continue
			}
			id := transcriptField(block, "id")
			call := pendingToolCall{
				target: target, started: when, session: sessionKey, id: id,
				name: name, model: model,
			}
			if server, tool, ok := auditutil.ParseMcpTool(name, "mcp__"); ok {
				call.mcpServer = server
				call.mcpTool = tool
			}
			record := call.record(when, "attempted")
			input, exists := block["input"]
			if exists {
				record.Object = auditutil.EncodeBoundedJSON(
					map[string]any{"input": auditutil.SanitizeToolInput(name, input)},
					64*1024,
				)
			}
			object.AddRecord(record)
			if id != "" {
				m.pending[id] = call
			}

		case "tool_result":
			id := transcriptField(block, "tool_use_id")
			call, exists := m.pending[id]
			if !exists {
				continue
			}
			delete(m.pending, id)
			outcome := "success"
			if failed, _ := block["is_error"].(bool); failed {
				outcome = "failure"
			}
			object.AddRecord(call.record(when, outcome))
		}
	}
}

func transcriptField(value map[string]any, names ...string) string {
	for _, name := range names {
		if text, ok := value[name].(string); ok && text != "" {
			return text
		}
	}
	return ""
}
