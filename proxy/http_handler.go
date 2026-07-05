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
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"

	"github.com/beego/beego/v2/core/logs"
	"github.com/casdoor/casdoor-aiguard/object"
)

// handlePlaintextConnection serves redirected, unencrypted HTTP traffic
// (typically port 80). It reads requests sequentially off the client
// connection, runs each through the decision pipeline, and either forwards
// it to origDst or writes back a synthetic deny response.
func (e *Engine) handlePlaintextConnection(br *bufio.Reader, clientConn net.Conn, origDst string, meta SourceMeta) {
	for {
		req, err := http.ReadRequest(br)
		if err != nil {
			return
		}

		host := requestHost(req, origDst)
		e.serveOneRequest(req, host, origDst, false, clientConn, meta)

		if req.Close {
			return
		}
	}
}

// handleTLSConnection serves redirected HTTPS traffic. It terminates TLS
// with a leaf certificate for the client's SNI (signed by aiguard's local
// CA), decrypts the HTTP payload for inspection, then re-encrypts a fresh
// TLS connection to the real destination to forward allowed requests.
func (e *Engine) handleTLSConnection(br *bufio.Reader, clientConn net.Conn, origDst string, meta SourceMeta) {
	peekedConn := &bufferedConn{Reader: br, Conn: clientConn}

	var sni string
	tlsConfig := &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			sni = hello.ServerName
			host := sni
			if host == "" {
				host, _, _ = net.SplitHostPort(origDst)
			}
			return e.ca.LeafCertificateFor(host)
		},
	}

	tlsServerConn := tls.Server(peekedConn, tlsConfig)
	defer tlsServerConn.Close()

	if err := tlsServerConn.Handshake(); err != nil {
		logs.Warn("proxy: TLS handshake with client failed: %v", err)
		return
	}

	upstreamHost := sni
	if upstreamHost == "" {
		upstreamHost, _, _ = net.SplitHostPort(origDst)
	}
	_, port, _ := net.SplitHostPort(origDst)
	if port == "" {
		port = "443"
	}

	serverBr := bufio.NewReader(tlsServerConn)
	for {
		req, err := http.ReadRequest(serverBr)
		if err != nil {
			return
		}

		host := requestHost(req, upstreamHost)
		e.serveOneRequest(req, host, net.JoinHostPort(upstreamHost, port), true, tlsServerConn, meta)

		if req.Close {
			return
		}
	}
}

// serveOneRequest runs the decision pipeline for one already-parsed request
// and either forwards it to (host, dialAddr) or writes a deny response back
// to clientConn.
func (e *Engine) serveOneRequest(req *http.Request, host string, dialAddr string, useTLS bool, clientConn net.Conn, meta SourceMeta) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(body))
	}

	action, reason := e.Decide(host, req, body, meta)

	if action == object.ActionDeny {
		writeDenyResponse(clientConn, reason)
		return
	}

	resp, err := forwardRequest(req, host, dialAddr, useTLS, body)
	if err != nil {
		logs.Warn("proxy: failed to forward request to %s: %v", host, err)
		writeUpstreamErrorResponse(clientConn, err)
		return
	}
	defer resp.Body.Close()

	resp.Write(clientConn)
}

// forwardRequest re-issues the (allowed) request against its real
// destination and returns the upstream response.
func forwardRequest(req *http.Request, host, dialAddr string, useTLS bool, body []byte) (*http.Response, error) {
	var conn net.Conn
	var err error
	if useTLS {
		conn, err = tls.Dial("tcp", dialAddr, &tls.Config{ServerName: host})
	} else {
		conn, err = net.Dial("tcp", dialAddr)
	}
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	outReq := req.Clone(req.Context())
	outReq.RequestURI = ""
	if outReq.URL.Host == "" {
		outReq.URL.Host = host
	}
	if useTLS {
		outReq.URL.Scheme = "https"
	} else {
		outReq.URL.Scheme = "http"
	}
	outReq.Body = io.NopCloser(bytes.NewReader(body))

	if err := outReq.Write(conn); err != nil {
		return nil, err
	}

	return http.ReadResponse(bufio.NewReader(conn), outReq)
}

func writeDenyResponse(conn net.Conn, reason string) {
	payload, _ := json.Marshal(map[string]string{
		"error":  "blocked_by_aiguard",
		"reason": reason,
	})
	resp := &http.Response{
		StatusCode:    http.StatusForbidden,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(payload)),
		ContentLength: int64(len(payload)),
	}
	resp.Write(conn)
}

func writeUpstreamErrorResponse(conn net.Conn, upstreamErr error) {
	payload, _ := json.Marshal(map[string]string{
		"error":  "upstream_unreachable",
		"detail": upstreamErr.Error(),
	})
	resp := &http.Response{
		StatusCode:    http.StatusBadGateway,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(payload)),
		ContentLength: int64(len(payload)),
	}
	resp.Write(conn)
}

func requestHost(req *http.Request, fallback string) string {
	if h := req.Host; h != "" {
		if host, _, err := net.SplitHostPort(h); err == nil {
			return host
		}
		return h
	}
	return fallback
}

// bufferedConn lets us hand a net.Conn whose first bytes were already
// consumed into a bufio.Reader (while peeking to distinguish TLS from
// plaintext) back to crypto/tls, which needs to read those same bytes.
type bufferedConn struct {
	Reader *bufio.Reader
	net.Conn
}

func (b *bufferedConn) Read(p []byte) (int, error) {
	return b.Reader.Read(p)
}
