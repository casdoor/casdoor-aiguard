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

//go:build !linux

package proxy

import "fmt"

// TransparentRedirect is a no-op stub on non-Linux platforms, where automatic
// iptables management is unavailable. aiguard still works as an explicit
// forward proxy there. The type mirrors the Linux implementation so main.go
// compiles unchanged.
type TransparentRedirect struct {
	proxyPort int
}

func NewTransparentRedirect(proxyPort int) *TransparentRedirect {
	return &TransparentRedirect{proxyPort: proxyPort}
}

func (t *TransparentRedirect) Install() error {
	return fmt.Errorf("automatic transparent redirect (iptables) is only supported on Linux")
}

func (t *TransparentRedirect) Remove() {}
