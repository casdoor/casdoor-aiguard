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

//go:build linux

package proxy

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/beego/beego/v2/core/logs"
)

// aiguardNatChain is a dedicated iptables chain so aiguard only ever touches
// its own rules and can clean up precisely without disturbing anything else.
const aiguardNatChain = "AIGUARD_OUTPUT"

// TransparentRedirect installs and removes the iptables NAT rules that redirect
// this host's egress HTTP(S) into aiguard's proxy. aiguard manages these rules
// itself so the operator does not have to run scripts/setup_iptables.sh by hand.
type TransparentRedirect struct {
	proxyPort  int
	excludeUID int
	installed  bool
}

// NewTransparentRedirect targets the given proxy port and excludes aiguard's
// own uid, so aiguard's forwarded upstream requests (and its calls to Casdoor)
// are not redirected back into aiguard — which would loop forever.
func NewTransparentRedirect(proxyPort int) *TransparentRedirect {
	return &TransparentRedirect{proxyPort: proxyPort, excludeUID: os.Getuid()}
}

func runIptables(args ...string) (string, error) {
	out, err := exec.Command("iptables", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Install lays down the redirect rules. It first clears any stale aiguard rules
// (e.g. left by a previous instance that was SIGKILLed) so installation is
// idempotent, then rolls back cleanly if any single rule fails.
func (t *TransparentRedirect) Install() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("root privileges required to manage iptables")
	}
	if _, err := exec.LookPath("iptables"); err != nil {
		return fmt.Errorf("iptables not found in PATH: %w", err)
	}

	// Idempotent: remove leftovers from a prior run before (re)installing.
	t.remove(false)

	port := strconv.Itoa(t.proxyPort)
	uid := strconv.Itoa(t.excludeUID)

	steps := [][]string{
		{"-t", "nat", "-N", aiguardNatChain},
		// aiguard's own outbound traffic must bypass the redirect, or the
		// requests it forwards on an agent's behalf would be redirected back
		// into itself in an infinite loop.
		{"-t", "nat", "-A", aiguardNatChain, "-m", "owner", "--uid-owner", uid, "-j", "RETURN"},
		// never redirect loopback traffic.
		{"-t", "nat", "-A", aiguardNatChain, "-d", "127.0.0.0/8", "-j", "RETURN"},
		// redirect egress HTTP and HTTPS into the proxy.
		{"-t", "nat", "-A", aiguardNatChain, "-p", "tcp", "--dport", "80", "-j", "REDIRECT", "--to-port", port},
		{"-t", "nat", "-A", aiguardNatChain, "-p", "tcp", "--dport", "443", "-j", "REDIRECT", "--to-port", port},
		// hook the chain into OUTPUT.
		{"-t", "nat", "-A", "OUTPUT", "-j", aiguardNatChain},
	}
	for _, s := range steps {
		if out, err := runIptables(s...); err != nil {
			t.remove(false) // roll back a partial install
			return fmt.Errorf("iptables %s failed: %v (%s)", strings.Join(s, " "), err, out)
		}
	}

	t.installed = true
	logs.Info("aiguard: transparent redirect enabled (egress tcp/80,443 -> 127.0.0.1:%s, excluding own uid %s)", port, uid)
	return nil
}

// Remove tears the redirect rules down, restoring the host's networking.
func (t *TransparentRedirect) Remove() {
	t.remove(true)
}

func (t *TransparentRedirect) remove(logIt bool) {
	// Detach the chain from OUTPUT (guard against it being linked more than once).
	for i := 0; i < 16; i++ {
		if _, err := runIptables("-t", "nat", "-C", "OUTPUT", "-j", aiguardNatChain); err != nil {
			break // no (more) references
		}
		if _, err := runIptables("-t", "nat", "-D", "OUTPUT", "-j", aiguardNatChain); err != nil {
			break
		}
	}
	runIptables("-t", "nat", "-F", aiguardNatChain)
	runIptables("-t", "nat", "-X", aiguardNatChain)
	if logIt && t.installed {
		logs.Info("aiguard: transparent redirect disabled, iptables restored")
	}
	t.installed = false
}
