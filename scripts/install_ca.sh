#!/usr/bin/env bash
# Copyright 2025 The Casdoor Authors. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# Installs aiguard's local CA certificate into this host's system trust store
# so that agents whose TLS is transparently terminated (MITM) by aiguard see a
# trusted certificate instead of an error. Run this on any host or bake it into
# any container base image that runs governed agents. Must run as root.
# Reverse with uninstall_ca.sh.
#
# Usage: sudo ./install_ca.sh [path/to/aiguard-ca.crt]
#
#   path  the CA cert to install (default: ./certs/aiguard-ca.crt, i.e. the one
#         aiguard generated on first run; you can also download it from the Web UI)

set -euo pipefail

CA_SRC="${1:-./certs/aiguard-ca.crt}"

if [[ $EUID -ne 0 ]]; then
	echo "This script must be run as root (it writes to the system trust store)." >&2
	exit 1
fi

if [[ ! -f "${CA_SRC}" ]]; then
	echo "CA certificate not found at '${CA_SRC}'." >&2
	echo "Start aiguard once to generate it, or download it from the aiguard Web UI, then pass its path." >&2
	exit 1
fi

if command -v update-ca-certificates >/dev/null 2>&1; then
	# Debian / Ubuntu / Alpine
	DEST="/usr/local/share/ca-certificates/aiguard-ca.crt"
	install -m 0644 "${CA_SRC}" "${DEST}"
	update-ca-certificates
	echo "Installed aiguard CA into the system trust store (${DEST})."
elif command -v update-ca-trust >/dev/null 2>&1; then
	# RHEL / CentOS / Fedora
	DEST="/etc/pki/ca-trust/source/anchors/aiguard-ca.crt"
	install -m 0644 "${CA_SRC}" "${DEST}"
	update-ca-trust extract
	echo "Installed aiguard CA into the system trust store (${DEST})."
else
	echo "Could not find update-ca-certificates or update-ca-trust on this host." >&2
	echo "Manually add '${CA_SRC}' to your distribution's CA trust store." >&2
	exit 1
fi

echo "Note: language runtimes with their own bundled trust stores (Node.js via"
echo "NODE_EXTRA_CA_CERTS, Python/requests via REQUESTS_CA_BUNDLE, Go via SSL_CERT_FILE)"
echo "may need the CA pointed at them separately."
