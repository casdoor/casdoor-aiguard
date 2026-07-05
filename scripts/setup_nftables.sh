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
# nftables equivalent of setup_iptables.sh, for hosts that manage firewall
# rules with nft instead. Must run as root. Reverse with cleanup_nftables.sh.
#
# Usage: sudo ./setup_nftables.sh [proxy_port] [uid_to_exclude]

set -euo pipefail

PROXY_PORT="${1:-9090}"
EXCLUDE_UID="${2:-0}"
TABLE="aiguard"

if [[ $EUID -ne 0 ]]; then
	echo "This script must be run as root (it edits nftables rules)." >&2
	exit 1
fi

nft add table ip "${TABLE}"
nft add chain ip "${TABLE}" output "{ type nat hook output priority -100 ; }"
nft flush chain ip "${TABLE}" output

nft add rule ip "${TABLE}" output meta skuid "${EXCLUDE_UID}" return
nft add rule ip "${TABLE}" output ip daddr 127.0.0.0/8 return
nft add rule ip "${TABLE}" output tcp dport { 80, 443 } redirect to :"${PROXY_PORT}"

echo "Done. Egress HTTP/HTTPS traffic (except uid ${EXCLUDE_UID} and loopback) is now redirected to 127.0.0.1:${PROXY_PORT}."
echo "Remember to trust aiguard's CA certificate on this host (download it from the aiguard Web UI)."
echo "To undo: sudo ./cleanup_nftables.sh"
