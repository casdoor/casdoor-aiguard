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
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type codexTOMLSetting struct {
	key   string
	value string
}

func readTOMLFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeTOMLObject(data, path)
}

func decodeTOMLObject(data []byte, path string) (map[string]any, error) {
	settings := map[string]any{}
	if len(strings.TrimSpace(string(data))) == 0 {
		return settings, nil
	}
	if err := toml.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return settings, nil
}

// updateCodexConfigTOML edits only the two active-model keys and AIGuard's
// provider table. It deliberately works on the original lines instead of
// marshaling the whole document, so MCP tables, profiles, comments and inline
// formatting survive unchanged.
func updateCodexConfigTOML(data []byte, model, baseURL string) ([]byte, error) {
	if _, err := decodeTOMLObject(data, "Codex config.toml"); err != nil {
		return nil, err
	}

	newline := "\n"
	if strings.Contains(string(data), "\r\n") {
		newline = "\r\n"
	}
	lines := splitTOMLLines(string(data))
	rootSettings := []codexTOMLSetting{
		{key: "model_provider", value: strconv.Quote(codexProvider)},
		{key: "model", value: strconv.Quote(model)},
	}
	providerSettings := []codexTOMLSetting{
		{key: "name", value: strconv.Quote("AIGuard")},
		{key: "base_url", value: strconv.Quote(baseURL)},
		{key: "wire_api", value: strconv.Quote("responses")},
		{key: "requires_openai_auth", value: "true"},
	}

	rootFound := map[string]bool{}
	providerFound := map[string]bool{}
	currentTable := ""
	providerHeader := -1
	for index, line := range lines {
		if table, ok := tomlTableName(line); ok {
			currentTable = table
			if table == "model_providers."+codexProvider {
				providerHeader = index
			}
			continue
		}
		key, ok := tomlAssignmentKey(line)
		if !ok {
			continue
		}
		if currentTable == "" {
			if setting, ok := findTOMLSetting(rootSettings, key); ok {
				updated, err := replaceTOMLValue(line, setting.value)
				if err != nil {
					return nil, fmt.Errorf("edit top-level %s: %w", key, err)
				}
				lines[index] = updated
				rootFound[key] = true
			}
			continue
		}
		if currentTable == "model_providers."+codexProvider {
			if setting, ok := findTOMLSetting(providerSettings, key); ok {
				updated, err := replaceTOMLValue(line, setting.value)
				if err != nil {
					return nil, fmt.Errorf("edit AIGuard provider %s: %w", key, err)
				}
				lines[index] = updated
				providerFound[key] = true
			}
		}
	}

	var missingProvider []string
	for _, setting := range providerSettings {
		if !providerFound[setting.key] {
			missingProvider = append(missingProvider, setting.key+" = "+setting.value+newline)
		}
	}
	if providerHeader >= 0 {
		if _, ending := splitTOMLLineEnding(lines[providerHeader]); ending == "" {
			lines[providerHeader] += newline
		}
		lines = insertTOMLLines(lines, providerHeader+1, missingProvider)
	} else {
		if len(lines) != 0 {
			last := len(lines) - 1
			if !strings.HasSuffix(lines[last], "\n") {
				lines[last] += newline
			}
			if strings.TrimSpace(lines[last]) != "" {
				lines = append(lines, newline)
			}
		}
		lines = append(lines, "[model_providers."+codexProvider+"]"+newline)
		lines = append(lines, missingProvider...)
	}

	var missingRoot []string
	for _, setting := range rootSettings {
		if !rootFound[setting.key] {
			missingRoot = append(missingRoot, setting.key+" = "+setting.value+newline)
		}
	}
	if len(missingRoot) != 0 {
		missingRoot = append(missingRoot, newline)
		lines = append(missingRoot, lines...)
	}

	updated := []byte(strings.Join(lines, ""))
	settings, err := decodeTOMLObject(updated, "updated Codex config.toml")
	if err != nil {
		return nil, err
	}
	if settings["model_provider"] != codexProvider || settings["model"] != model {
		return nil, fmt.Errorf("updated Codex active model settings are invalid")
	}
	providers, ok := settings["model_providers"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("updated Codex model_providers is not a table")
	}
	provider, ok := providers[codexProvider].(map[string]any)
	if !ok || provider["name"] != "AIGuard" || provider["base_url"] != baseURL ||
		provider["wire_api"] != "responses" || provider["requires_openai_auth"] != true {
		return nil, fmt.Errorf("updated AIGuard provider is invalid")
	}
	return updated, nil
}

func splitTOMLLines(document string) []string {
	if document == "" {
		return nil
	}
	lines := strings.SplitAfter(document, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func insertTOMLLines(lines []string, index int, inserted []string) []string {
	result := make([]string, 0, len(lines)+len(inserted))
	result = append(result, lines[:index]...)
	result = append(result, inserted...)
	return append(result, lines[index:]...)
}

func findTOMLSetting(settings []codexTOMLSetting, key string) (codexTOMLSetting, bool) {
	for _, setting := range settings {
		if setting.key == key {
			return setting, true
		}
	}
	return codexTOMLSetting{}, false
}

func tomlTableName(line string) (string, bool) {
	body, _ := splitTOMLLineEnding(line)
	trimmed := strings.TrimSpace(body)
	if !strings.HasPrefix(trimmed, "[") {
		return "", false
	}
	if strings.HasPrefix(trimmed, "[[") {
		end := strings.Index(trimmed[2:], "]]")
		if end < 0 {
			return "", false
		}
		return "[]" + strings.TrimSpace(trimmed[2:2+end]), true
	}
	end := strings.IndexByte(trimmed, ']')
	if end < 0 {
		return "", false
	}
	suffix := strings.TrimSpace(trimmed[end+1:])
	if suffix != "" && !strings.HasPrefix(suffix, "#") {
		return "", false
	}
	return strings.TrimSpace(trimmed[1:end]), true
}

func tomlAssignmentKey(line string) (string, bool) {
	body, _ := splitTOMLLineEnding(line)
	trimmed := strings.TrimSpace(body)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "[") {
		return "", false
	}
	equals := strings.IndexByte(body, '=')
	if equals < 0 {
		return "", false
	}
	key := strings.TrimSpace(body[:equals])
	if key == "" || strings.ContainsAny(key, ".\"'") {
		return "", false
	}
	return key, true
}

func replaceTOMLValue(line, value string) (string, error) {
	body, ending := splitTOMLLineEnding(line)
	equals := strings.IndexByte(body, '=')
	if equals < 0 {
		return "", fmt.Errorf("assignment is missing =")
	}
	start := equals + 1
	for start < len(body) && (body[start] == ' ' || body[start] == '\t') {
		start++
	}
	comment := tomlCommentStart(body, start)
	end := len(body)
	if comment >= 0 {
		end = comment
	}
	for end > start && (body[end-1] == ' ' || body[end-1] == '\t') {
		end--
	}
	oldValue := body[start:end]
	if strings.Contains(oldValue, `"""`) || strings.Contains(oldValue, `'''`) {
		return "", fmt.Errorf("multiline values are not supported for managed keys")
	}
	return body[:start] + value + body[end:] + ending, nil
}

func tomlCommentStart(line string, start int) int {
	inBasic, inLiteral, escaped := false, false, false
	for index := start; index < len(line); index++ {
		character := line[index]
		if inBasic {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
			} else if character == '"' {
				inBasic = false
			}
			continue
		}
		if inLiteral {
			if character == '\'' {
				inLiteral = false
			}
			continue
		}
		switch character {
		case '"':
			inBasic = true
		case '\'':
			inLiteral = true
		case '#':
			return index
		}
	}
	return -1
}

func splitTOMLLineEnding(line string) (string, string) {
	if strings.HasSuffix(line, "\r\n") {
		return line[:len(line)-2], "\r\n"
	}
	if strings.HasSuffix(line, "\n") {
		return line[:len(line)-1], "\n"
	}
	return line, ""
}
