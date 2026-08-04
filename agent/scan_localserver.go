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
	"os"
	"strings"

	"github.com/casdoor/casdoor-aiguard/localserver"
)

// localServerInstallMethod labels how these installations got onto the host. A
// server-discovered row only survives the deduplication against the layout
// scans when its executable sits outside every packaged layout - which is what
// a binary built and run straight out of a source checkout looks like, and the
// reason this scan exists. The port is how the installation was found, not how
// it was installed, so it is not what the row is labelled with.
const localServerInstallMethod = "source"

// scanLocalServers reports an installation for every agent that answers on one
// of its default loopback ports. It is the one scan that starts from a running
// agent rather than from a directory layout, which is how it finds an
// installation the packaged layouts do not cover - a binary run out of a source
// checkout, say.
//
// The path reported is the executable of the process holding the port, so the
// row lands in the Agents table as something the patcher can act on; a listener
// whose process cannot be resolved is dropped rather than reported with no real
// path. The version comes from the running agent itself, which knows it better
// than its binary does; fillMissingVersions still reads the binary for the
// agents whose server will not say. Finding, identifying and questioning the
// server is the localserver package's job.
func scanLocalServers() []Installation {
	var installations []Installation
	for _, fingerprint := range compiledFingerprints {
		if fingerprint.LocalServer == nil {
			continue
		}
		mark := len(installations)
		for _, port := range fingerprint.LocalServer.Ports {
			installations = append(installations, scanLocalServerPort(fingerprint, port)...)
		}
		stampAgentId(installations, mark, fingerprint.ID)
		fillMissingVersions(installations, mark, fingerprint)
	}
	return installations
}

func scanLocalServerPort(fingerprint *compiledFingerprint, port int) []Installation {
	base, ok := fingerprint.LocalServer.Confirm(port)
	if !ok {
		return nil
	}
	// Sanitized like every other version the scan reports, so a server that
	// says "v2.87.0" lands in the table as 2.87.0 next to the rest.
	version := sanitizeVersion(fingerprint.LocalServer.Version(base))

	var result []Installation
	seen := map[string]bool{}
	for _, process := range localserver.Listeners(port) {
		// One server answering on both IPv4 and IPv6 is one installation.
		key := strings.ToLower(process.Path)
		if process.Path == "" || seen[key] {
			continue
		}
		seen[key] = true
		if info, err := os.Stat(process.Path); err != nil || !info.Mode().IsRegular() {
			continue
		}
		result = append(result, Installation{
			Name: fingerprint.DisplayName, Version: version, Path: process.Path,
			InstallMethod: localServerInstallMethod, Owner: process.Owner,
		})
	}
	return result
}
