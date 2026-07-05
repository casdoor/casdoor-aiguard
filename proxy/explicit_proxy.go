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
	"bufio"
	"net"
	"net/http"

	"github.com/beego/beego/v2/core/logs"
)

// handleExplicitProxy serves a client that has been pointed at aiguard as a
// forward proxy (e.g. via HTTP_PROXY / HTTPS_PROXY). This path needs neither
// root nor iptables/SO_ORIGINAL_DST, so it lets the full recognize -> decide ->
// enforce pipeline be exercised end-to-end on any OS (Windows/macOS dev hosts
// included). In production on Linux the transparent redirect path is preferred
// because it requires no agent-side configuration at all.
func (e *Engine) handleExplicitProxy(br *bufio.Reader, clientConn net.Conn, meta SourceMeta) {
	for {
		req, err := http.ReadRequest(br)
		if err != nil {
			return
		}

		// HTTPS: the client asks us to open a tunnel, then starts its own TLS
		// handshake through it. We MITM that handshake with a leaf cert from
		// our local CA, exactly as in the transparent TLS path.
		if req.Method == http.MethodConnect {
			if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
				return
			}
			// req.Host is "host:port"; reuse the same reader so any TLS bytes
			// the client already pipelined are not lost.
			e.handleTLSConnection(br, clientConn, req.Host, meta)
			return
		}

		// Plain HTTP proxying: the request arrives in absolute-form, e.g.
		// "POST http://api.example.com/v1/payments HTTP/1.1".
		host := req.URL.Hostname()
		if host == "" {
			host = requestHost(req, "")
		}
		port := req.URL.Port()
		if port == "" {
			port = "80"
		}
		if host == "" {
			logs.Warn("proxy: explicit-proxy request without a resolvable host, dropping")
			return
		}

		e.serveOneRequest(req, host, net.JoinHostPort(host, port), false, clientConn, meta)

		if req.Close {
			return
		}
	}
}
