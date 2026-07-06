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
	"net/http"
	"time"

	"github.com/casdoor/casdoor-aiguard/recognizers"
	"github.com/google/uuid"
)

// maxCaptureBytes caps how much of a request or response body is stored per
// event, so an unusually large payload (or a long streaming LLM reply) can't
// blow up memory or the audit log.
const maxCaptureBytes = 256 * 1024

// Event is one intercepted, recognized (or passed-through) egress request,
// shown in the dashboard's event stream and written to the audit log. Beyond
// the decision, it captures the raw HTTP exchange (method, path, query, request
// body, and the upstream response) so an operator can see exactly what was sent
// and what came back.
type Event struct {
	Id            string              `json:"id"`
	Timestamp     time.Time           `json:"timestamp"`
	SourcePid     int                 `json:"sourcePid,omitempty"`
	SourceProcess string              `json:"sourceProcess,omitempty"`
	Agent         string              `json:"agent,omitempty"`
	Destination   string              `json:"destination"`
	Recognizer    string              `json:"recognizer,omitempty"`
	Intent        *recognizers.Intent `json:"intent,omitempty"`
	Decision      Action              `json:"decision"`
	Reason        string              `json:"reason"`

	// HTTP exchange capture.
	Method         string `json:"method,omitempty"`
	Path           string `json:"path,omitempty"`
	Query          string `json:"query,omitempty"`
	RequestBody    string `json:"requestBody,omitempty"`
	ResponseStatus int    `json:"responseStatus,omitempty"`
	ResponseBody   string `json:"responseBody,omitempty"`
}

func NewEvent(destination string, sourcePid int, sourceProcess string, recognizerName string, intent *recognizers.Intent, decision Action, reason string) *Event {
	return &Event{
		Id:            uuid.NewString(),
		Timestamp:     time.Now(),
		SourcePid:     sourcePid,
		SourceProcess: sourceProcess,
		Destination:   destination,
		Recognizer:    recognizerName,
		Intent:        intent,
		Decision:      decision,
		Reason:        reason,
	}
}

// AttachRequest records the intercepted request's method, URL path, query
// string and (capped) body on the event.
func (e *Event) AttachRequest(req *http.Request, body []byte) {
	if req != nil {
		e.Method = req.Method
		if req.URL != nil {
			e.Path = req.URL.Path
			e.Query = req.URL.RawQuery
		}
	}
	e.RequestBody = truncateBody(body)
}

// AttachResponse records the upstream response's status and (already-capped)
// body on the event.
func (e *Event) AttachResponse(status int, body string) {
	e.ResponseStatus = status
	e.ResponseBody = body
}

func truncateBody(b []byte) string {
	if len(b) > maxCaptureBytes {
		return string(b[:maxCaptureBytes]) + "\n...[truncated]"
	}
	return string(b)
}
