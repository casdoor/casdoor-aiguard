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

package proxy

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/beego/beego/v2/core/logs"
	"github.com/casdoor/casdoor-aiguard/conf"
	"github.com/casdoor/casdoor-aiguard/object"
)

// ManageTransparentRedirect installs the iptables transparent redirect (when
// enabled and running as root on Linux) and arranges to remove it on graceful
// shutdown, restoring the host's networking. aiguard excludes its own uid so
// its forwarded/upstream traffic isn't redirected back into itself.
//
// SIGKILL/crash cannot be caught by any process, so the next startup clears
// leftover rules idempotently (see TransparentRedirect.Install). On non-Linux
// hosts, or when not root, it logs a warning and leaves aiguard running as an
// explicit forward proxy.
func ManageTransparentRedirect() {
	if !conf.AutoTransparentProxy() {
		return
	}

	proxyPort := object.GetSettings().Intercept.ProxyPort
	redirect := NewTransparentRedirect(proxyPort)
	if err := redirect.Install(); err != nil {
		logs.Warn("aiguard: transparent redirect not enabled (%v). Running as an explicit forward proxy instead: point agents at HTTP_PROXY/HTTPS_PROXY=http://127.0.0.1:%d, or run aiguard as root on Linux.", err, proxyPort)
		return
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-stop
		logs.Info("aiguard: received %v, restoring network rules and shutting down...", sig)
		redirect.Remove()
		os.Exit(0)
	}()
}
