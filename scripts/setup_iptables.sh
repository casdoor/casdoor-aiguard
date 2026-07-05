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
# Redirects outbound (egress) HTTP/HTTPS traffic on this host into aiguard's
# transparent proxy, so intercepted traffic never has to touch AI agent
# code or configuration. Must run as root. Reverse with cleanup_iptables.sh.
#
# Usage: sudo ./setup_iptables.sh [proxy_port] [uid_to_exclude]
#
#   proxy_port      port aiguard's proxy is listening on (default 9090)
#   uid_to_exclude  the uid aiguard itself runs as, so its own outbound
#                    connections (e.g. to Casdoor) aren't redirected back
#                    into itself (default: uid of "root", override if
#                    aiguard runs as a dedicated service user)

set -euo pipefail

PROXY_PORT="${1:-9090}"
EXCLUDE_UID="${2:-0}"
CHAIN="AIGUARD_OUTPUT"

if [[ $EUID -ne 0 ]]; then
	echo "This script must be run as root (it edits iptables NAT rules)." >&2
	exit 1
fi

echo "Setting up transparent redirection of egress HTTP(S) into aiguard on port ${PROXY_PORT}..."

iptables -t nat -N "${CHAIN}" 2>/dev/null || iptables -t nat -F "${CHAIN}"

# Don't redirect aiguard's own outbound traffic (e.g. calls to Casdoor),
# or it would try to intercept itself.
iptables -t nat -A "${CHAIN}" -m owner --uid-owner "${EXCLUDE_UID}" -j RETURN

# Don't redirect traffic already destined for the proxy or for loopback.
iptables -t nat -A "${CHAIN}" -d 127.0.0.0/8 -j RETURN

# Redirect egress HTTP and HTTPS to aiguard's transparent proxy.
iptables -t nat -A "${CHAIN}" -p tcp --dport 80 -j REDIRECT --to-port "${PROXY_PORT}"
iptables -t nat -A "${CHAIN}" -p tcp --dport 443 -j REDIRECT --to-port "${PROXY_PORT}"

# Hook the chain into OUTPUT if not already present.
if ! iptables -t nat -C OUTPUT -j "${CHAIN}" 2>/dev/null; then
	iptables -t nat -A OUTPUT -j "${CHAIN}"
fi

echo "Done. Egress HTTP/HTTPS traffic (except uid ${EXCLUDE_UID} and loopback) is now redirected to 127.0.0.1:${PROXY_PORT}."
echo "Remember to trust aiguard's CA certificate on this host (download it from the aiguard Web UI)."
echo "To undo: sudo ./cleanup_iptables.sh"
