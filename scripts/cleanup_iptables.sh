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
# Removes the redirection rules installed by setup_iptables.sh. Must run as root.

set -euo pipefail

CHAIN="AIGUARD_OUTPUT"

if [[ $EUID -ne 0 ]]; then
	echo "This script must be run as root (it edits iptables NAT rules)." >&2
	exit 1
fi

iptables -t nat -D OUTPUT -j "${CHAIN}" 2>/dev/null || true
iptables -t nat -F "${CHAIN}" 2>/dev/null || true
iptables -t nat -X "${CHAIN}" 2>/dev/null || true

echo "aiguard iptables redirection rules removed."
