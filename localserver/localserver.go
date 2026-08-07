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

// Package localserver finds a program by the loopback port its HTTP server
// answers on. It knows nothing about what it is looking for: Confirm asks
// whoever holds a port to identify itself, Listeners names the process holding
// it, and Version asks a confirmed server what it is running.
//
// The first two are both needed because neither is enough. An open port
// identifies nothing - any process can hold any port - so only the server's own
// answer tells one program from another. And an answer names no file on disk,
// so only the socket's owning process says where the program that gave it
// lives.
package localserver

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// A loopback port either answers immediately or is not there, so the dial
	// is kept short: callers run this on a page load.
	dialTimeout    = 300 * time.Millisecond
	requestTimeout = 2 * time.Second
	// maxResponseBody bounds how much of a response is read, since what is on
	// the other end of the port is not yet known.
	maxResponseBody = 64 * 1024
)

// probeHosts are the only addresses ever dialled. This package looks for a
// server on the host it runs on; it never scans a network.
var probeHosts = []string{"127.0.0.1", "::1"}

// Server describes one program's local HTTP server: where to look for it, and
// how to tell it apart from anything else that could be holding its port.
type Server struct {
	// Ports are the TCP ports the program listens on out of the box. A program
	// reconfigured onto another port is out of this package's reach.
	Ports []int `json:"ports,omitempty"`
	// ProbePath is the endpoint the confirmation request asks for. It must
	// answer without authentication, since this package holds no credentials,
	// and should be the cheapest such endpoint the program exposes.
	ProbePath string `json:"probePath,omitempty"`
	// ProbeMarkers are case-insensitive substrings of the probe response - its
	// headers and body together - that only this program emits. Any one of them
	// confirms the listener; with none of them present the port is left alone.
	ProbeMarkers []string `json:"probeMarkers,omitempty"`

	// VersionPath is an endpoint answering JSON that carries the version the
	// program reports about itself, and VersionFields walks that JSON to the
	// version string - {"data", "version"} for a payload of
	// {"data": {"version": "v2.87.0"}}. A running program is the most
	// authoritative source there is for its own version, which is why it is
	// asked before the binary on disk is inspected.
	//
	// Both are optional, and the version is only as available as the endpoint
	// is: one that turns out to need credentials, or that answers something
	// other than a version string, yields "" and leaves the caller to fall back
	// to whatever the binary itself records.
	VersionPath   string   `json:"versionPath,omitempty"`
	VersionFields []string `json:"versionFields,omitempty"`
}

// Process is a process holding a listening TCP port.
type Process struct {
	Pid   int
	Path  string
	Owner string
}

// Confirm reports whether this server is the one listening on port, and on
// which base URL it answered so a caller can go on asking it things. The dial
// is only a cheap way to skip a port nothing holds; the probe that follows is
// what tells the program apart from anything else that could be sitting on its
// default port.
func (s *Server) Confirm(port int) (base string, ok bool) {
	if s == nil || s.ProbePath == "" || len(s.ProbeMarkers) == 0 {
		return "", false
	}
	for _, host := range probeHosts {
		address := net.JoinHostPort(host, strconv.Itoa(port))
		conn, err := net.DialTimeout("tcp", address, dialTimeout)
		if err != nil {
			continue
		}
		conn.Close()
		base = "http://" + address
		if s.probe(base) {
			return base, true
		}
	}
	return "", false
}

// probe asks the server at base to identify itself. Headers and body are
// searched together, because the marker that identifies a program most reliably
// is often a header - a session cookie it names after itself - that rides on
// every response including the error ones. The status code is not part of the
// test for the same reason: a program having a bad day is still that program.
func (s *Server) probe(base string) bool {
	answer, ok := get(base + s.ProbePath)
	if !ok {
		return false
	}

	haystack := strings.ToLower(answer.header + string(answer.body))
	for _, marker := range s.ProbeMarkers {
		if marker != "" && strings.Contains(haystack, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

// Version asks the server at base - one Confirm has already identified - what
// version it is running, and returns "" when the answer is not a version. An
// endpoint that needs credentials this package does not have answers an error
// payload rather than a version, and falls out here the same way a missing
// endpoint does, so a caller with another source for the version can use it.
func (s *Server) Version(base string) string {
	if s == nil || s.VersionPath == "" || len(s.VersionFields) == 0 {
		return ""
	}
	answer, ok := get(base + s.VersionPath)
	if !ok {
		return ""
	}

	var payload any
	if json.Unmarshal(answer.body, &payload) != nil {
		return ""
	}
	for _, field := range s.VersionFields {
		object, ok := payload.(map[string]any)
		if !ok {
			return ""
		}
		payload = object[field]
	}
	version, _ := payload.(string)
	return version
}

// answer is as much of an HTTP response as this package looks at.
type answer struct {
	header string
	body   []byte
}

// get makes the one kind of request this package ever makes: a plain GET at a
// loopback address, following no redirect, reading no more than it has to.
func get(url string) (answer, bool) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return answer{}, false
	}
	client := &http.Client{
		Timeout: requestTimeout,
		// A program is identified by what it answers itself, so a redirect
		// somewhere else is not an answer.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		return answer{}, false
	}
	defer response.Body.Close()

	var header strings.Builder
	_ = response.Header.Write(&header)
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBody))
	if err != nil {
		return answer{}, false
	}
	return answer{header: header.String(), body: body}, true
}
