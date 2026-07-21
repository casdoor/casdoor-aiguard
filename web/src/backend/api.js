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

async function request(path, options) {
  const resp = await fetch(path, {
    headers: {"Content-Type": "application/json"},
    ...options,
  });
  const body = await resp.json();
  if (body.status !== "ok") {
    throw new Error(body.msg || "request failed");
  }
  return body.data;
}

export function getEvents(limit = 200) {
  return request(`/api/events?limit=${limit}`);
}

export function getAgents() {
  return request("/api/agents");
}

// target is {agentId, path, owner}, straight from a row of the agents table.
export function patchAgent(target) {
  return request("/api/agents/patch", {method: "POST", body: JSON.stringify(target)});
}

export function unpatchAgent(target) {
  return request("/api/agents/unpatch", {method: "POST", body: JSON.stringify(target)});
}

export function getRecords(agent = "", limit = 200) {
  return request(`/api/records?agent=${encodeURIComponent(agent)}&limit=${limit}`);
}

export function getPolicy() {
  return request("/api/policy");
}

export function updatePolicy(policy) {
  return request("/api/policy", {method: "POST", body: JSON.stringify(policy)});
}

// The Policy Hub's policy sets are read from aiguard's policy hub directory on
// every call, so a JSON file dropped in there shows up without a restart.
export function getPolicySets() {
  return request("/api/policy-sets");
}

export function getPolicySet(name) {
  return request(`/api/policy-set?name=${encodeURIComponent(name)}`);
}

// Enables or disables a policy set for live interception on patched agents. The
// server re-checks the same gate the card shows, so enabling a set the host
// cannot enforce is rejected with the reason.
export function setPolicySetEnabled(name, enabled) {
  return request("/api/policy-set/enable", {method: "POST", body: JSON.stringify({name, enabled})});
}

export function getSettings() {
  return request("/api/settings");
}

export function updateSettings(settings) {
  return request("/api/settings", {method: "POST", body: JSON.stringify(settings)});
}

export function caCertDownloadUrl() {
  return "/api/ca-cert";
}
