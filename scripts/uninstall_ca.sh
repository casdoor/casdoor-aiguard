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
# Removes aiguard's local CA certificate from this host's system trust store,
# undoing install_ca.sh. Must run as root.
#
# Usage: sudo ./uninstall_ca.sh

set -euo pipefail

if [[ $EUID -ne 0 ]]; then
	echo "This script must be run as root (it edits the system trust store)." >&2
	exit 1
fi

removed=0

if [[ -f /usr/local/share/ca-certificates/aiguard-ca.crt ]]; then
	rm -f /usr/local/share/ca-certificates/aiguard-ca.crt
	command -v update-ca-certificates >/dev/null 2>&1 && update-ca-certificates --fresh
	removed=1
fi

if [[ -f /etc/pki/ca-trust/source/anchors/aiguard-ca.crt ]]; then
	rm -f /etc/pki/ca-trust/source/anchors/aiguard-ca.crt
	command -v update-ca-trust >/dev/null 2>&1 && update-ca-trust extract
	removed=1
fi

if [[ "${removed}" -eq 1 ]]; then
	echo "Removed aiguard CA from the system trust store."
else
	echo "No aiguard CA certificate was found in the system trust store; nothing to do."
fi
