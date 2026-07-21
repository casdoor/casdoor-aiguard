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

// Package mcpserver is aiguard's Model Context Protocol server, the form
// aiguard takes when it instruments an agent that has no hook system.
//
// Claude Desktop is the case in point: it is a packaged application with no
// hook directory and no CLI, and the only extension point its user can
// configure is the list of MCP servers in claude_desktop_config.json. So the
// patch registers the aiguard binary itself there, and Claude Desktop launches
// this server over stdio. Everything it then sees - the session handshake, the
// tools the model asks for, the calls it makes - is posted to the running
// aiguard as records.
//
// Why a separate process at all, rather than serving MCP from the aiguard that
// is already running? Because claude_desktop_config.json accepts stdio servers
// and nothing else. The app validates each entry with, in effect:
//
//	(entry.type === undefined || entry.type === "stdio") && typeof entry.command === "string"
//
// and then keeps only command, args and env. An entry pointing at a URL fails
// that check and is dropped silently - no error, it simply never connects. And
// stdio means the client spawns the server and owns the pipe, so there is no
// way to attach an already-running process to it. Claude Desktop's remote MCP
// support lives in its Connectors UI, which a patch that works by editing files
// on disk cannot reach.
//
// Being spawned by the agent rather than by an operator shapes the rest of the
// design: stdout carries the protocol and nothing else (a stray log line there
// breaks the session), no configuration is read from disk because the working
// directory belongs to the agent, and a failure to reach aiguard is never
// allowed to take the agent's MCP session down with it. Records travel back to
// the running aiguard over HTTP rather than being written to the log directly,
// so that the process serving the Records page stays the only writer.
package mcpserver

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	serverName = "casdoor-aiguard"
	// fallbackProtocolVersion is used only when a client opens a session
	// without naming a version; otherwise the client's own version is echoed
	// back, which is how MCP negotiates.
	fallbackProtocolVersion = "2025-06-18"
	// maxLineBytes bounds one protocol message. MCP frames each message as a
	// single line of JSON, and tool results can be large.
	maxLineBytes  = 8 * 1024 * 1024
	reportTimeout = 5 * time.Second
	// recordTimeFormat keeps milliseconds, which is what separates events that
	// happen within the same second of a session handshake.
	recordTimeFormat = "2006-01-02T15:04:05.000Z07:00"
	// reportQueueDepth is how many records may be waiting to be posted before
	// new ones are dropped instead of stalling the session.
	reportQueueDepth = 256
)

// Server speaks MCP over a pair of streams and reports what it sees.
type Server struct {
	in         io.Reader
	out        io.Writer
	agentId    string
	recordsUrl string

	// writeMutex serializes protocol writes, since the reporter runs alongside
	// the read loop.
	writeMutex sync.Mutex
	client     *http.Client

	// Records go out through a single worker rather than a goroutine per event.
	// Posting them concurrently would let them arrive shuffled, and an audit
	// trail out of sequence is worse than one that lags: aiguard stores records
	// in arrival order, so arrival order has to be event order.
	queue     chan map[string]any
	reporting sync.WaitGroup
}

// Subcommand is the argument an agent's registered MCP server entry passes to
// the aiguard binary to reach this package.
const Subcommand = "mcp-server"

// ServeIfInvoked serves an MCP session and exits when this binary was launched
// as an agent's MCP server, and returns immediately when it was not.
//
// It never returns in the serving case, because the two modes have nothing in
// common: an MCP session owns stdin and stdout and has no use for the proxy or
// the management API, so there is nothing for the caller to carry on with.
func ServeIfInvoked() {
	if len(os.Args) < 2 || os.Args[1] != Subcommand {
		return
	}
	if err := Run(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "aiguard %s: %v\n", Subcommand, err)
		os.Exit(1)
	}
	os.Exit(0)
}

// Run parses the subcommand's arguments and serves MCP on stdin/stdout until
// the agent closes the connection.
func Run(args []string) error {
	flags := flag.NewFlagSet("mcp-server", flag.ContinueOnError)
	// Diagnostics must not touch stdout, which belongs to the protocol.
	flags.SetOutput(os.Stderr)
	recordsUrl := flags.String("records-url", "", "aiguard endpoint to post behaviour records to")
	agentId := flags.String("agent", "claude-desktop", "id of the agent this server was registered with")
	if err := flags.Parse(args); err != nil {
		return err
	}

	server := &Server{
		in:         os.Stdin,
		out:        os.Stdout,
		agentId:    *agentId,
		recordsUrl: *recordsUrl,
		client:     &http.Client{Timeout: reportTimeout},
		queue:      make(chan map[string]any, reportQueueDepth),
	}
	return server.Serve()
}

// Serve reads one JSON-RPC message per line, records it, and answers the ones
// that expect an answer.
func (s *Server) Serve() error {
	s.startReporter()
	s.report("session", "start", nil)
	defer func() {
		s.report("session", "end", nil)
		// Drain the queue before returning. This process exits the moment the
		// agent closes stdin, so without waiting, every queued record dies here.
		close(s.queue)
		s.reporting.Wait()
	}()

	scanner := bufio.NewScanner(s.in)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var message request
		if err := json.Unmarshal(line, &message); err != nil {
			// A message we cannot parse is still worth recording: it is
			// evidence of an agent doing something unexpected.
			s.report("mcp", "malformed", map[string]any{"error": err.Error()})
			continue
		}

		s.report("mcp", message.Method, message.paramsMap())

		// A JSON-RPC notification carries no id and takes no reply.
		if len(message.Id) == 0 {
			continue
		}
		if err := s.write(s.dispatch(message)); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// dispatch answers one request. Unknown methods get a proper JSON-RPC error
// rather than silence, so a client probing for optional capabilities moves on
// instead of waiting.
func (s *Server) dispatch(message request) response {
	switch message.Method {
	case "initialize":
		return message.reply(s.initializeResult(message))
	case "ping":
		return message.reply(map[string]any{})
	case "tools/list":
		return message.reply(map[string]any{"tools": tools})
	case "tools/call":
		return s.callTool(message)
	default:
		return message.fail(methodNotFound, fmt.Sprintf("unsupported method %q", message.Method))
	}
}

func (s *Server) initializeResult(message request) map[string]any {
	// Echo the client's protocol version back when it names one - that is how
	// MCP settles on a shared version.
	version := fallbackProtocolVersion
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if json.Unmarshal(message.Params, &params) == nil && params.ProtocolVersion != "" {
		version = params.ProtocolVersion
	}

	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": serverName, "version": Version},
	}
}

// Version is the version this server reports to agents. It is a var so a build
// can stamp it.
var Version = "0.1.0"

func (s *Server) callTool(message request) response {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(message.Params, &params); err != nil {
		return message.fail(invalidParams, err.Error())
	}
	if params.Name != reportActionTool {
		return message.fail(invalidParams, fmt.Sprintf("unknown tool %q", params.Name))
	}

	s.report("tool", params.Name, params.Arguments)

	action, _ := params.Arguments["action"].(string)
	return message.reply(map[string]any{
		"content": []map[string]any{{
			"type": "text",
			"text": fmt.Sprintf("Recorded %q in the Casdoor AIGuard audit log.", action),
		}},
	})
}

// report queues one record for aiguard. It never blocks the caller, so an
// aiguard that is slow or gone cannot slow the agent down.
func (s *Server) report(eventType, action string, payload map[string]any) {
	if s.recordsUrl == "" {
		return
	}

	object := ""
	if payload != nil {
		if encoded, err := json.Marshal(payload); err == nil {
			object = string(encoded)
		}
	}
	record := map[string]any{
		"agent": s.agentId,
		// Stamped here, before the post is queued, and to millisecond precision:
		// reports race each other to aiguard, so arrival order is not event
		// order and only this timestamp can put them back in sequence.
		"createdTime": time.Now().Format(recordTimeFormat),
		"eventType":   eventType,
		"action":      action,
		"object":      object,
	}

	select {
	case s.queue <- record:
	default:
		// aiguard is not keeping up. Drop the record rather than stall the
		// agent's session waiting on an audit log.
	}
}

// startReporter posts queued records one at a time, in the order they were
// recorded. Every failure is swallowed: an aiguard that is stopped or
// unreachable must not disturb the agent's session.
func (s *Server) startReporter() {
	s.reporting.Add(1)
	go func() {
		defer s.reporting.Done()
		for record := range s.queue {
			body, err := json.Marshal(record)
			if err != nil {
				continue
			}
			resp, err := s.client.Post(s.recordsUrl, "application/json", bytes.NewReader(body))
			if err != nil {
				continue
			}
			resp.Body.Close()
		}
	}()
}

func (s *Server) write(message response) error {
	message.JSONRPC = "2.0"
	encoded, err := json.Marshal(message)
	if err != nil {
		return err
	}

	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	_, err = s.out.Write(append(encoded, '\n'))
	return err
}
