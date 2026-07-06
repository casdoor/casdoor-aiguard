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

	action, reason, event := e.Decide(host, req, body, meta)

	if action == object.ActionDeny {
		writeDenyResponse(clientConn, reason)
		event.AttachResponse(http.StatusForbidden, reason)
		object.RecordEvent(event)
		return
	}

	resp, err := forwardRequest(req, host, dialAddr, useTLS, body)
	if err != nil {
		logs.Warn("proxy: failed to forward request to %s: %v", host, err)
		writeUpstreamErrorResponse(clientConn, err)
		event.AttachResponse(http.StatusBadGateway, err.Error())
		object.RecordEvent(event)
		return
	}
	defer resp.Body.Close()

	// Tee the response body into a capped buffer as it streams back to the
	// client, so the upstream reply is captured for the audit record without
	// buffering it up front (which would stall SSE / streaming LLM responses).
	capture := &captureBuffer{limit: maxCaptureBytes}
	resp.Body = &teeReadCloser{Reader: io.TeeReader(resp.Body, capture), closer: resp.Body}

	resp.Write(clientConn)

	event.AttachResponse(resp.StatusCode, capture.String())
	object.RecordEvent(event)
}

// maxCaptureBytes caps how much of a streaming response body is retained for
// the audit record; it mirrors the request-body cap in the object package.
const maxCaptureBytes = 256 * 1024

// captureBuffer is an io.Writer that retains at most `limit` bytes of what is
// written through it (the rest is counted as written but discarded), so tee-ing
// a large streaming response can't grow memory without bound.
type captureBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (c *captureBuffer) Write(p []byte) (int, error) {
	if remaining := c.limit - c.buf.Len(); remaining > 0 {
		if remaining < len(p) {
			c.buf.Write(p[:remaining])
			c.truncated = true
		} else {
			c.buf.Write(p)
		}
	} else if len(p) > 0 {
		c.truncated = true
	}
	return len(p), nil
}

func (c *captureBuffer) String() string {
	if c.truncated {
		return c.buf.String() + "\n...[truncated]"
	}
	return c.buf.String()
}

// teeReadCloser reads through an io.TeeReader while delegating Close to the
// original body, so the upstream connection is still reaped when the response
// body is closed.
type teeReadCloser struct {
	io.Reader
	closer io.Closer
}

func (t *teeReadCloser) Close() error { return t.closer.Close() }

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
		conn.Close()
		return nil, err
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), outReq)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// The response body streams lazily off conn, so conn must stay open
	// until the caller has finished reading it (which matters for chunked /
	// SSE streaming responses like the LLM APIs return). Tie conn's close to
	// the body's Close so the caller's `defer resp.Body.Close()` reaps both.
	resp.Body = &connClosingBody{ReadCloser: resp.Body, conn: conn}
	return resp, nil
}

// connClosingBody ties the upstream connection's lifetime to the response
// body: closing the body also closes the connection it streams from.
type connClosingBody struct {
	io.ReadCloser
	conn net.Conn
}

func (b *connClosingBody) Close() error {
	err := b.ReadCloser.Close()
	b.conn.Close()
	return err
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
