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

// Package proxy is aiguard's interception engine: a user-space transparent
// proxy that egress traffic is redirected into (via iptables/nftables
// REDIRECT, see scripts/), which terminates TLS with a locally-generated CA
// so the plaintext HTTP payload can be handed to the recognizers package.
package proxy

import (
	"bufio"
	"fmt"
	"net"

	"github.com/beego/beego/v2/core/logs"
	"github.com/casdoor/casdoor-aiguard/casdoorclient"
	"github.com/casdoor/casdoor-aiguard/object"
	"github.com/casdoor/casdoor-aiguard/recognizers"
)

// Engine is the running transparent proxy: one listener accepts both
// redirected plaintext HTTP and redirected TLS connections (distinguished by
// peeking the first byte), terminates TLS itself, runs the decision
// pipeline, and forwards allowed requests to their real destination.
type Engine struct {
	ca       *CertificateAuthority
	registry *recognizers.Registry
	pdp      *casdoorclient.Client
}

func NewEngine(ca *CertificateAuthority) *Engine {
	return &Engine{
		ca:       ca,
		registry: recognizers.Default(),
		pdp:      casdoorclient.NewClient(),
	}
}

// Start listens on the configured proxy port and serves forever. Call it in a
// goroutine; it does not return unless the listener fails.
func (e *Engine) Start() error {
	port := object.GetSettings().Intercept.ProxyPort
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("proxy: failed to listen on port %d: %w", port, err)
	}
	logs.Info("aiguard proxy listening on :%d (transparent redirect + explicit-proxy)", port)
	return e.Serve(ln)
}

// Serve accepts and handles connections on ln until it errors. It is factored
// out of Start so tests can drive the full pipeline over an ephemeral port.
func (e *Engine) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			logs.Warn("proxy: accept error: %v", err)
			return err
		}
		go e.handleConnection(conn)
	}
}

func (e *Engine) handleConnection(conn net.Conn) {
	defer conn.Close()

	pid, procName, agent := LookupSourceProcess(conn.RemoteAddr())
	meta := SourceMeta{Pid: pid, ProcessName: procName, Agent: agent}

	br := bufio.NewReader(conn)

	// Prefer the transparent path: if this connection arrived via an
	// iptables/nftables REDIRECT, SO_ORIGINAL_DST tells us where the agent
	// really meant to go, and the agent has no idea aiguard exists.
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		if origDst, err := getOriginalDestination(tcpConn); err == nil {
			firstByte, peekErr := br.Peek(1)
			if peekErr != nil {
				return
			}
			// TLS records start with content type 0x16 (handshake).
			if firstByte[0] == 0x16 {
				e.handleTLSConnection(br, conn, origDst, meta)
			} else {
				e.handlePlaintextConnection(br, conn, origDst, meta)
			}
			return
		}
	}

	// No transparent redirect (non-Linux dev hosts, or a client pointed at us
	// with HTTP(S)_PROXY). Fall back to serving as an explicit forward proxy so
	// the full decision pipeline can still be exercised end-to-end.
	e.handleExplicitProxy(br, conn, meta)
}
