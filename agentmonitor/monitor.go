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

import "errors"

// Start brings up every agent monitor: the Windows-only Cowork transcript
// monitor and the cross-platform Codex rollout monitor. Each is started
// independently so a failure in one still lets the other run; the combined
// error (if any) is returned for logging.
func Start() error {
	return errors.Join(startCoworkMonitor(), codexMonitor.start())
}

// Stop tears both monitors down.
func Stop() {
	stopCoworkMonitor()
	codexMonitor.stopMonitor()
}
