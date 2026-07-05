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

	"github.com/casdoor/casdoor-aiguard/recognizers"
	"github.com/google/uuid"
)

// Event is one intercepted, recognized (or passed-through) egress request,
// shown in the dashboard's event stream and written to the audit log.
type Event struct {
	Id            string              `json:"id"`
	Timestamp     time.Time           `json:"timestamp"`
	SourcePid     int                 `json:"sourcePid,omitempty"`
	SourceProcess string              `json:"sourceProcess,omitempty"`
	Destination   string              `json:"destination"`
	Recognizer    string              `json:"recognizer,omitempty"`
	Intent        *recognizers.Intent `json:"intent,omitempty"`
	Decision      Action              `json:"decision"`
	Reason        string              `json:"reason"`
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
