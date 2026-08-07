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

package agent

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	pathpkg "path"
	"sort"
	"strings"

	"github.com/casdoor/casdoor-aiguard/localserver"
)

// The agent registry ships as data, one file per agent, embedded into the
// binary. Fingerprint is its schema; adding an agent aiguard can recognize is
// adding a file here and nothing else.
//
//go:embed fingerprints/*.json
var fingerprintFS embed.FS

const fingerprintDir = "fingerprints"

// fingerprints is the registry of agents aiguard knows how to recognize.
var fingerprints = mustLoadFingerprints(fingerprintFS)

// mustLoadFingerprints panics on malformed registry data. The data is embedded,
// so a failure here is a broken build rather than a bad host: there is no
// runtime input that can cause it and nothing sensible to fall back to - an
// aiguard that has silently forgotten how to recognize an agent is worse than
// one that will not start. TestFingerprintsLoad turns that panic into a test
// failure before it can reach a release.
func mustLoadFingerprints(files fs.FS) []Fingerprint {
	loaded, err := loadFingerprints(files)
	if err != nil {
		panic("agent: cannot load agent fingerprints: " + err.Error())
	}
	return loaded
}

// loadFingerprints reads every fingerprint file in name order, which is by ID
// since a file is named after the agent it describes. Order decides nothing:
// the identification rules of two agents never overlap - the tests pin exactly
// which path belongs to which agent - and installations are re-sorted by owner
// and path before they are reported. It is fixed only so that two builds of the
// same data behave identically.
//
// Everything the Go compiler used to check about the registry is checked here
// instead, and every failure is fatal rather than skipped: a fingerprint file
// silently dropped for a typo would leave aiguard blind to an agent while still
// reporting success, which is the one outcome this package cannot allow.
func loadFingerprints(files fs.FS) ([]Fingerprint, error) {
	names, err := fs.Glob(files, fingerprintDir+"/*.json")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("no fingerprint files under %s/", fingerprintDir)
	}

	loaded := make([]Fingerprint, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		fingerprint, err := decodeFingerprint(files, name)
		if err != nil {
			return nil, err
		}
		if seen[fingerprint.ID] {
			return nil, fmt.Errorf("%s: duplicate agent id %q", name, fingerprint.ID)
		}
		seen[fingerprint.ID] = true
		loaded = append(loaded, fingerprint)
	}
	return loaded, nil
}

// decodeFingerprint reads one file strictly: an unknown field is an error
// rather than a silently ignored line, because in a hand-edited data file a
// field name that matches nothing is a misspelling of one that would have
// mattered.
func decodeFingerprint(files fs.FS, name string) (Fingerprint, error) {
	data, err := fs.ReadFile(files, name)
	if err != nil {
		return Fingerprint{}, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var fingerprint Fingerprint
	if err := decoder.Decode(&fingerprint); err != nil {
		return Fingerprint{}, fmt.Errorf("%s: %w", name, err)
	}
	if decoder.More() {
		return Fingerprint{}, fmt.Errorf("%s: trailing content after the fingerprint object", name)
	}
	if err := validateFingerprint(fingerprint, name); err != nil {
		return Fingerprint{}, fmt.Errorf("%s: %w", name, err)
	}
	return fingerprint, nil
}

// validateFingerprint rejects data that would make detection dead or dangerous.
// The two failure modes are opposites and both matter: a fingerprint that can
// never match leaves an agent undetected, while one that matches everything -
// an empty marker, since every string contains "" - would attribute unrelated
// processes on the host to an agent, which is how a scan turns into a false
// accusation.
func validateFingerprint(f Fingerprint, name string) error {
	if id := strings.TrimSuffix(pathpkg.Base(name), ".json"); f.ID != id {
		// The file name is the registry's index: it is how a person finds the
		// entry for an agent, and what keeps two files from claiming one ID.
		return fmt.Errorf("id %q does not match the file name %q", f.ID, id)
	}
	if f.DisplayName == "" {
		return errors.New("displayName is required")
	}

	for i, marker := range f.CmdMarkers {
		if strings.TrimSpace(marker) == "" {
			return fmt.Errorf("cmdMarkers[%d] is empty, which would match every process", i)
		}
	}
	for i, rule := range f.ExtraExecRules {
		if rule.isEmpty() {
			return fmt.Errorf("extraExecRules[%d] constrains nothing, so it can never match", i)
		}
		for j, contains := range rule.Contains {
			if contains == "" {
				return fmt.Errorf("extraExecRules[%d].contains[%d] is empty, which would match every path", i, j)
			}
		}
	}
	if err := validateLocalServer(f.LocalServer); err != nil {
		return err
	}

	// A fingerprint is only worth shipping if some scan can act on it. These are
	// the three kinds of evidence: where the agent sits on disk, what its
	// process looks like, and what its server answers.
	if len(deriveExecRules(f)) == 0 && len(f.CmdMarkers) == 0 && f.LocalServer == nil {
		return errors.New("no exec rules, cmdline markers or local server: this agent could never be found")
	}
	return nil
}

// validateLocalServer requires a declared server to be complete enough for
// localserver.Confirm to accept it. Confirm already refuses a half-filled one,
// so an incomplete block is not unsafe - it is simply inert, which in a data
// file reads as a working probe and is worth failing on.
func validateLocalServer(server *localserver.Server) error {
	if server == nil {
		return nil
	}
	if len(server.Ports) == 0 {
		return errors.New("localServer.ports is required")
	}
	if server.ProbePath == "" || len(server.ProbeMarkers) == 0 {
		return errors.New("localServer needs both probePath and probeMarkers to confirm anything")
	}
	for i, marker := range server.ProbeMarkers {
		if strings.TrimSpace(marker) == "" {
			return fmt.Errorf("localServer.probeMarkers[%d] is empty, which confirms nothing", i)
		}
	}
	return nil
}
