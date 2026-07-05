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

package util

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/beego/beego/v2/core/logs"
)

// RuntimeEnv returns a human-readable description of the OS the process is
// running on, distinguishing a Windows host from Linux running inside WSL.
func RuntimeEnv() string {
	env := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/proc/version"); err == nil {
			v := strings.ToLower(string(data))
			if strings.Contains(v, "microsoft") || strings.Contains(v, "wsl") {
				return env + " (WSL)"
			}
		}
	}
	return env
}

// LogRuntimeEnv prints the current runtime environment so it is clear whether
// aiguard is running on a Windows host or inside WSL.
func LogRuntimeEnv() {
	logs.Info("aiguard running on %s", RuntimeEnv())
}
