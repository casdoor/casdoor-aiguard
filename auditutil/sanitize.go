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

// Package auditutil contains the privacy boundary shared by agent collectors.
// Values must be sanitized and bounded here before being sent or persisted.
package auditutil

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	credentialPattern = regexp.MustCompile(`(?i)\b(?:sk-(?:ant-|proj-)?[a-z0-9_-]{12,}|gh[pousr]_[a-z0-9]{20,}|github_pat_[a-z0-9_]{20,}|AKIA[0-9A-Z]{16}|AIza[0-9a-z_-]{30,}|xox[baprs]-[0-9a-z-]{12,}|eyJ[a-z0-9_-]{10,}\.[a-z0-9_-]{10,}\.[a-z0-9_-]{10,})\b`)
	bearerPattern     = regexp.MustCompile(`(?i)(bearer\s+)[a-z0-9._~+/=-]{12,}`)
	privateKeyPattern = regexp.MustCompile(`(?s)-----BEGIN [^-\n]*PRIVATE KEY-----.*?-----END [^-\n]*PRIVATE KEY-----`)
)

// SanitizeValue recursively redacts fields whose names identify credentials,
// as well as recognizable token and private-key forms embedded in strings.
func SanitizeValue(key string, value any) any {
	if SensitiveKey(key) {
		return "[REDACTED]"
	}
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for childKey, child := range typed {
			result[childKey] = SanitizeValue(childKey, child)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, child := range typed {
			result[i] = SanitizeValue("", child)
		}
		return result
	case string:
		return SanitizeString(typed)
	default:
		return value
	}
}

// SensitiveKey recognizes common credential-bearing field names after
// punctuation and case normalization.
func SensitiveKey(key string) bool {
	normalized := strings.ToLower(key)
	normalized = strings.NewReplacer("_", "", "-", "", ".", "").Replace(normalized)
	if normalized == "token" || normalized == "accesstoken" || normalized == "refreshtoken" || normalized == "idtoken" {
		return true
	}
	for _, marker := range []string{"secret", "token", "password", "passwd", "credential", "privatekey", "apikey", "authorization", "cookie"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

// SanitizeString redacts credential formats that may appear inside an error,
// prompt, command or other non-secret-named field.
func SanitizeString(value string) string {
	value = privateKeyPattern.ReplaceAllString(value, "[REDACTED PRIVATE KEY]")
	value = bearerPattern.ReplaceAllString(value, "${1}[REDACTED]")
	return credentialPattern.ReplaceAllString(value, "[REDACTED]")
}

// IsSensitiveRead identifies file-read operations whose result must be hidden
// in full even if its individual fields do not look like secrets.
func IsSensitiveRead(toolName string, input any) bool {
	if toolName != "Read" && toolName != "read_file" && !strings.HasSuffix(toolName, "__read_file") {
		return false
	}
	object, ok := input.(map[string]any)
	if !ok {
		return false
	}
	path := stringValue(object["file_path"])
	if path == "" {
		path = stringValue(object["path"])
	}
	normalized := strings.ToLower(strings.ReplaceAll(filepath.ToSlash(path), `\`, "/"))
	base := filepath.Base(normalized)
	if strings.HasPrefix(base, ".env") && base != ".env.example" && base != ".env.sample" && base != ".env.template" {
		return true
	}
	if strings.HasPrefix(normalized, ".ssh/") || strings.Contains(normalized, "/.ssh/") || normalized == ".aws/credentials" || strings.HasSuffix(normalized, "/.aws/credentials") {
		return true
	}
	if base == ".npmrc" || base == ".pypirc" || base == "credentials" || base == "id_rsa" || base == "id_ed25519" {
		return true
	}
	return strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key")
}

// EncodeBoundedJSON serializes a sanitized payload and replaces oversized
// values with a small metadata envelope and preview. The preview is safe
// because callers sanitize the value before invoking this function.
func EncodeBoundedJSON(value any, maximum int) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	if len(encoded) <= maximum {
		return string(encoded)
	}
	previewBytes := encoded[:maximum/3]
	truncated, err := json.Marshal(map[string]any{
		"truncated":     true,
		"originalBytes": len(encoded),
		"preview":       string(previewBytes),
	})
	if err != nil {
		return ""
	}
	return string(truncated)
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
