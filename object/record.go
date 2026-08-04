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

package object

import (
	"time"

	"github.com/google/uuid"
)

// maxRecordObjectBytes caps the payload stored per record. Patched agents
// already trim what they send; this is the server-side backstop.
const maxRecordObjectBytes = 64 * 1024

// maxRecordTitleBytes is the server-side backstop for Title, mirroring
// maxRecordObjectBytes: a reporter is expected to send a short label, not
// truncate it correctly, so aiguard still caps it on the way in.
const maxRecordTitleBytes = 512

// Record is one behaviour-log entry reported by a patched agent, modelled on
// Casdoor's object.Record so the two audit trails read the same way.
//
// It is the counterpart to Event, and the distinction matters: an Event is
// traffic aiguard intercepted from the outside and ruled on, while a Record is
// what the agent says it did, pushed from a hook aiguard installed inside it.
// Records cover behaviour that never touches the network - a session reset, a
// local tool call - which interception cannot see at all.
type Record struct {
	Id          string `json:"id"`
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	CreatedTime string `json:"createdTime"`

	// Which patched agent reported this, and from where.
	Agent     string `json:"agent"`
	AgentPath string `json:"agentPath,omitempty"`
	ClientIp  string `json:"clientIp,omitempty"`

	// What happened. EventType and Action mirror the agent's own event
	// vocabulary (for OpenClaw, "message"/"received"), so a record stays
	// recognizable to someone reading the agent's documentation.
	EventType string `json:"eventType"`
	Action    string `json:"action"`
	// Outcome is a normalized result shared by agent hook records:
	// attempted, success, failure or denied. It stays empty for lifecycle
	// events where a result would be misleading.
	Outcome    string `json:"outcome,omitempty"`
	SessionKey string `json:"sessionKey,omitempty"`
	User       string `json:"user,omitempty"`
	Channel    string `json:"channel,omitempty"`

	// Title is the agent's own short label for the session it reported this
	// record from - e.g. Claude Code's session-picker title. It is optional
	// and only as fresh as the agent last reported it; the Sessions page
	// falls back to a guess when a session has none.
	Title string `json:"title,omitempty"`

	// Correlation and operation identity emitted by agent hooks. Keeping them as
	// first-class fields makes the Records page useful without coupling it to a
	// particular hook's raw payload shape.
	PromptId  string `json:"promptId,omitempty"`
	ToolUseId string `json:"toolUseId,omitempty"`
	ToolName  string `json:"toolName,omitempty"`
	McpServer string `json:"mcpServer,omitempty"`
	McpTool   string `json:"mcpTool,omitempty"`

	// Model and duration are retained when the hook supplies them. No model API
	// request or response bodies are collected.
	Model      string `json:"model,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`

	// Object is the event payload as JSON, and Detail a human-readable note.
	Object string `json:"object,omitempty"`
	Detail string `json:"detail,omitempty"`

	// IsTriggered marks a record that matched a policy rule, mirroring
	// Casdoor's field of the same name.
	IsTriggered bool `json:"isTriggered"`

	// The Casbin triple the /enforce path ruled on: Resource is the object
	// ("host#tool" for an MCP tools/call) and Intent the action category. They
	// are empty for a record that was only logged, and they are what makes a
	// record correctable - a self-learned rule is written about exactly this
	// pair, so without them there is nothing to learn from.
	Resource string `json:"resource,omitempty"`
	Intent   string `json:"intent,omitempty"`

	// The verdict, filled in by the /enforce path. PolicySet names the set that
	// ruled on the operation and is empty when no set did - either because the
	// record was only logged, or because nothing enabled applied to it. Reason
	// explains a block in one line, and is empty for an allow.
	PolicySet string `json:"policySet,omitempty"`
	IsAllowed bool   `json:"isAllowed"`
	Reason    string `json:"reason,omitempty"`

	// The operator's correction of the verdict above: "allow" or "deny" when a
	// person disagreed with what aiguard decided, empty while nobody has said
	// anything. This is the only field of a record a human writes, and it is the
	// input the self-learned policy set is derived from - a blocked call marked
	// "allow" becomes a rule saying that call should have gone through.
	Feedback     string `json:"feedback,omitempty"`
	FeedbackBy   string `json:"feedbackBy,omitempty"`
	FeedbackTime string `json:"feedbackTime,omitempty"`
	// Correctable is the API representation of IsCorrectable, so clients do
	// not have to duplicate the resource-and-intent rule.
	Correctable bool `json:"correctable"`
}

// The three states of a record's Feedback field. FeedbackNone is the empty
// string rather than a word, so a record nobody has corrected serializes
// without the field at all.
const (
	FeedbackNone  = ""
	FeedbackAllow = "allow"
	FeedbackDeny  = "deny"
)

// IsCorrectable reports whether a record can carry feedback at all. Only the
// operations the /enforce path ruled on can: a record that was merely logged
// has no Casbin triple to write a corrected rule about.
func (r *Record) IsCorrectable() bool {
	return r.Resource != "" && r.Intent != ""
}

// normalize fills in the fields a reporting agent is not expected to supply and
// enforces the payload cap. An agent posts what it knows about the event; the
// identity and timestamp of the record are aiguard's to assign.
func (r *Record) normalize() {
	if r.Id == "" {
		r.Id = uuid.NewString()
	}
	if r.Name == "" {
		r.Name = r.Id
	}
	if r.Owner == "" {
		r.Owner = r.Agent
	}
	if r.CreatedTime == "" {
		// Milliseconds, matching what the agents report: several events from one
		// burst routinely share a second, and the Records page orders on this.
		r.CreatedTime = time.Now().Format("2006-01-02T15:04:05.000Z07:00")
	}
	// Only the /enforce path rules on an operation. Everything else reaching the
	// log is behaviour the agent already performed, so it counts as allowed.
	if !r.IsTriggered {
		r.IsAllowed = true
	}
	r.Correctable = r.IsCorrectable()
	if len(r.Object) > maxRecordObjectBytes {
		r.Object = r.Object[:maxRecordObjectBytes] + "\n...[truncated]"
	}
	if len(r.Title) > maxRecordTitleBytes {
		r.Title = r.Title[:maxRecordTitleBytes] + "..."
	}
}
