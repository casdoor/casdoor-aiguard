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

// Package auditutil contains the credential-redaction and size boundary shared
// by agent collectors. What an agent did is worth recording in full; the
// secrets that happen to pass through it are not, so values are stripped of
// recognizable credentials and bounded here before being sent or persisted.
//
// "Recognizable" is doing real work in that sentence: SanitizeString and
// SanitizeValue match known credential *formats* (bearer tokens, cloud/API
// key shapes, PEM private key blocks) against a fixed set of patterns - this
// is not a general PII or secrets scanner, and was never meant to be one. A
// password typed in plain language, an ID or account number, or an internal
// secret that does not happen to match one of those formats passes through
// untouched. Callers that hand this package free text an agent's user
// actually wrote - as opposed to short, structured tool arguments - are
// relying on it for exactly the same narrow guarantee: known formats caught,
// nothing else promised. See SanitizeString for the specifics.
package auditutil

import (
	"encoding/json"
	"path"
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
// prompt, command or other non-secret-named field: a PEM private key block,
// a "Bearer <token>" header, or one of a handful of well-known API key
// shapes (OpenAI/Anthropic sk-..., GitHub tokens, AWS AKIA..., Google
// AIza..., Slack xox..., a bare JWT). Nothing outside those specific formats
// is touched - this is pattern matching against known credential shapes, not
// content-aware secret or PII detection, and it makes no attempt to
// recognize a password, an ID number or a company's own internal token
// scheme. A caller passing this whatever an agent's user actually typed -
// OpenAgent's "message" records, most notably - is relying on it to catch
// only what these patterns catch, not on it having reviewed the text for
// sensitivity in any broader sense.
func SanitizeString(value string) string {
	value = privateKeyPattern.ReplaceAllString(value, "[REDACTED PRIVATE KEY]")
	value = bearerPattern.ReplaceAllString(value, "${1}[REDACTED]")
	return credentialPattern.ReplaceAllString(value, "[REDACTED]")
}

// ParseMcpTool splits the canonical MCP tool name used by Claude hooks and
// transcripts: mcp__<server>__<tool>.
func ParseMcpTool(name, prefix string) (string, string, bool) {
	if prefix == "" || !strings.HasPrefix(name, prefix) {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(name, prefix), "__", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// SanitizeToolInput hides content written to a sensitive file while retaining
// the operation metadata needed to understand the audit record.
func SanitizeToolInput(toolName string, input any) any {
	sanitized := SanitizeValue("", input)
	if !isSensitiveWrite(toolName) || !hasSensitivePath(input) {
		return sanitized
	}
	inputMap, ok := sanitized.(map[string]any)
	if !ok {
		return sanitized
	}
	for key := range inputMap {
		normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
		switch normalized {
		case "content", "oldstring", "newstring":
			inputMap[key] = "[REDACTED: sensitive file content]"
		}
	}
	return inputMap
}

// IsSensitiveRead identifies file-read operations whose result must be hidden
// in full even if its individual fields do not look like secrets.
func IsSensitiveRead(toolName string, input any) bool {
	normalizedTool := strings.ToLower(toolName)
	if normalizedTool == "bash" || strings.HasSuffix(normalizedTool, "__bash") {
		inputMap, ok := input.(map[string]any)
		if !ok {
			return false
		}
		for _, token := range strings.Fields(stringValue(inputMap["command"])) {
			if isSensitivePath(strings.Trim(token, "\"'`;|&()<>")) {
				return true
			}
		}
		return false
	}
	if normalizedTool != "read" && normalizedTool != "read_file" && !strings.HasSuffix(normalizedTool, "__read_file") {
		return false
	}
	return hasSensitivePath(input)
}

func isSensitiveWrite(toolName string) bool {
	normalized := strings.ToLower(toolName)
	return normalized == "write" || normalized == "edit" || normalized == "write_file" || normalized == "edit_file" ||
		strings.HasSuffix(normalized, "__write_file") || strings.HasSuffix(normalized, "__edit_file")
}

func hasSensitivePath(input any) bool {
	inputMap, ok := input.(map[string]any)
	if !ok {
		return false
	}
	filePath := stringValue(inputMap["file_path"])
	if filePath == "" {
		filePath = stringValue(inputMap["path"])
	}
	return isSensitivePath(filePath)
}

func isSensitivePath(filePath string) bool {
	if filePath == "" {
		return false
	}
	normalized := strings.ToLower(strings.ReplaceAll(filePath, `\`, "/"))
	base := path.Base(normalized)
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
	preview := strings.ToValidUTF8(string(encoded[:maximum/3]), "")
	truncated, err := json.Marshal(map[string]any{
		"truncated":     true,
		"originalBytes": len(encoded),
		"preview":       preview,
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
