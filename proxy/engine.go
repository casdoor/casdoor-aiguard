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

// Start listens on conf.GetProxyPort() and serves forever. Call it in a
// goroutine; it does not return unless the listener fails.
func (e *Engine) Start() error {
	port := object.GetSettings().Intercept.ProxyPort
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("proxy: failed to listen on port %d: %w", port, err)
	}
	logs.Info("aiguard transparent proxy listening on :%d", port)

	for {
		conn, err := ln.Accept()
		if err != nil {
			logs.Warn("proxy: accept error: %v", err)
			continue
		}
		go e.handleConnection(conn)
	}
}

func (e *Engine) handleConnection(conn net.Conn) {
	defer conn.Close()

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		logs.Warn("proxy: non-TCP connection, dropping")
		return
	}

	origDst, err := getOriginalDestination(tcpConn)
	if err != nil {
		logs.Warn("proxy: could not determine original destination (is this connection reaching aiguard via an iptables/nftables REDIRECT? see scripts/): %v", err)
		return
	}

	pid, procName := LookupSourceProcess(tcpConn.RemoteAddr())
	meta := SourceMeta{Pid: pid, ProcessName: procName}

	br := bufio.NewReader(conn)
	firstByte, err := br.Peek(1)
	if err != nil {
		return
	}

	// TLS records start with content type 0x16 (handshake).
	if firstByte[0] == 0x16 {
		e.handleTLSConnection(br, conn, origDst, meta)
		return
	}

	e.handlePlaintextConnection(br, conn, origDst, meta)
}
