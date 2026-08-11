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

package util

import "os"

// carryOwnerForward is a no-op on Windows, matching patch/file_ownership_windows.go:
// os.Chown is unsupported there.
//
// Known limitation: a replaced file inherits its ACL from the parent directory
// instead of keeping its own, so a per-file ACE does not survive an
// AtomicWriteFile. Preserving it means reapplying the security descriptor
// through golang.org/x/sys/windows, which no caller has needed yet.
func carryOwnerForward(os.FileInfo, string) error { return nil }
