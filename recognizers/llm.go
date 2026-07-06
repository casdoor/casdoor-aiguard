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

package recognizers

import (
	"encoding/json"
	"net/http"
	"strings"
)

// llmChatBody is the request shape shared by the OpenAI-compatible chat APIs
// (OpenAI, DeepSeek, and every provider openclaw talks to via its
// "openai-completions" adapter) and, closely enough, the Anthropic messages
// API. Because the request already carries the model and the conversation
// verbatim, the intent (which model, asked what) is explicit - no per-agent
// guessing required.
type llmChatBody struct {
	Model    string          `json:"model"`
	Messages []llmMessage    `json:"messages"`
	Prompt   json.RawMessage `json:"prompt"` // legacy /completions: string or []string
	System   json.RawMessage `json:"system"` // anthropic top-level system prompt
	Stream   bool            `json:"stream"`
}

type llmMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string, or [{type,text},...] parts
}

// LLMRecognizer recognizes LLM chat/completion API calls - e.g.
// POST /v1/chat/completions (OpenAI, DeepSeek, ...), POST /v1/completions
// (legacy), and POST /v1/messages (Anthropic) - and extracts the model and the
// conversation so the specific question asked and the model used are recorded.
type LLMRecognizer struct{}

func (r *LLMRecognizer) Name() string { return "llm" }

func (r *LLMRecognizer) Recognize(host string, req *http.Request, body []byte) (*Intent, bool) {
	if req.Method != http.MethodPost {
		return nil, false
	}
	ct := req.Header.Get("Content-Type")
	if ct != "" && !strings.Contains(ct, "json") {
		return nil, false
	}

	path := req.URL.Path
	pathLooksLikeLLM := strings.Contains(path, "/chat/completions") ||
		strings.HasSuffix(path, "/completions") ||
		strings.HasSuffix(path, "/messages") ||
		strings.HasSuffix(path, "/responses")

	var b llmChatBody
	if err := json.Unmarshal(body, &b); err != nil {
		return nil, false
	}

	hasConversation := len(b.Messages) > 0 || len(b.Prompt) > 0
	// A recognizable LLM call needs a conversation payload. When the URL path
	// doesn't obviously name an LLM endpoint, also require a model so we don't
	// mistake an unrelated JSON body that happens to have a "messages" field.
	if !hasConversation {
		return nil, false
	}
	if !pathLooksLikeLLM && b.Model == "" {
		return nil, false
	}

	action := "chat.completion"
	switch {
	case strings.HasSuffix(path, "/messages"):
		action = "messages"
	case len(b.Messages) == 0 && len(b.Prompt) > 0:
		action = "text.completion"
	}

	prompt := lastUserPrompt(b.Messages)
	if prompt == "" && len(b.Prompt) > 0 {
		prompt = messageText(b.Prompt)
	}

	raw := map[string]interface{}{
		"model":    b.Model,
		"path":     path,
		"stream":   b.Stream,
		"messages": flattenMessages(b.Messages),
	}
	if system := messageText(b.System); system != "" {
		raw["system"] = system
	}

	return &Intent{
		Category: "llm.chat",
		Action:   action,
		Model:    b.Model,
		Prompt:   prompt,
		Raw:      raw,
	}, true
}

// lastUserPrompt returns the content of the final user-role message, which is
// the concrete question the agent just asked the model.
func lastUserPrompt(messages []llmMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messageText(messages[i].Content)
		}
	}
	return ""
}

// flattenMessages turns each message into {role, content} with content
// normalized to plain text, for a readable audit record of the conversation.
func flattenMessages(messages []llmMessage) []map[string]string {
	out := make([]map[string]string, 0, len(messages))
	for _, m := range messages {
		out = append(out, map[string]string{
			"role":    m.Role,
			"content": messageText(m.Content),
		})
	}
	return out
}

// messageText normalizes an LLM content field to plain text. Content may be a
// JSON string, or an array of typed parts (e.g. [{"type":"text","text":"..."}])
// as used by vision/multimodal messages; anything else falls back to its raw
// JSON so nothing is silently dropped from the audit record.
func messageText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}

	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var sb strings.Builder
		for _, p := range parts {
			if p.Text == "" {
				continue
			}
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(p.Text)
		}
		if sb.Len() > 0 {
			return sb.String()
		}
	}

	return string(raw)
}
