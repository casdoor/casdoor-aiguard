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

// Operator login is optional: the Casdoor connection may be unconfigured or
// down, in which case the UI just stays anonymous.
export function getAuthConfig() {
  return request("/api/auth-config");
}

// Resolves to null when nobody is signed in.
export function getAccount() {
  return request("/api/account");
}

export function signin(code, state) {
  return request(`/api/signin?code=${encodeURIComponent(code)}&state=${encodeURIComponent(state)}`, {method: "POST"});
}

export function signout() {
  return request("/api/signout", {method: "POST"});
}

// The machine aiguard is installed on - the UI shows it to make clear that an
// aiguard instance guards one specific host.
export function getHostInfo() {
  return request("/api/host-info");
}

export function getEvents(limit = 200) {
  return request(`/api/events?limit=${limit}`);
}

export function getAgents() {
  return request("/api/agents");
}

// Reads only the active API configuration for one discovered agent. The
// response says whether a key exists, but never includes the key itself.
export function getAgentLlmApi(target) {
  const params = new URLSearchParams({
    agentId: target.agentId,
    path: target.path,
    owner: target.owner,
  });
  return request(`/api/agents/llm-api?${params.toString()}`);
}

export function updateAgentLlmApi(config) {
  return request("/api/agents/llm-api", {method: "POST", body: JSON.stringify(config)});
}

// target is {agentId, path, owner}, straight from a row of the agents table.
export function patchAgent(target) {
  return request("/api/agents/patch", {method: "POST", body: JSON.stringify(target)});
}

export function unpatchAgent(target) {
  return request("/api/agents/unpatch", {method: "POST", body: JSON.stringify(target)});
}

export function getRecords(agent = "", limit = 200, eventType = "", outcome = "", session = "") {
  const params = new URLSearchParams({agent, limit: String(limit), eventType, outcome, session});
  return request(`/api/records?${params.toString()}`);
}

// One row per session, newest first - the data behind the Sessions page.
export function getSessions() {
  return request("/api/sessions");
}

// Corrects the verdict on one record: feedback is "allow", "deny", or "" to
// withdraw a correction. Resolves to {record, policySet}, because correcting a
// record is also what teaches the self-learned policy set.
export function setRecordFeedback(id, feedback) {
  return request("/api/records/feedback", {method: "POST", body: JSON.stringify({id, feedback})});
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

// The signed-in person's own policy set, stored in their Casdoor user's
// properties rather than on this host. Requires being signed in - unlike the
// rest of aiguard, an anonymous session has no digital employee to show.
export function getEmployeePolicySet() {
  return request("/api/employee-policy-set");
}

export function updateEmployeePolicySet(policySet) {
  return request("/api/employee-policy-set", {method: "POST", body: JSON.stringify(policySet)});
}

// The rules aiguard learned from the records this person corrected, rendered as
// a policy set. Read-only: it is written by giving feedback on the Records page,
// not by editing it here.
export function getLearnedPolicySet() {
  return request("/api/learned-policy-set");
}

// Forgets one learned rule and withdraws the record feedback it came from.
export function deleteLearnedRule(id) {
  return request("/api/learned-policy-set/delete", {method: "POST", body: JSON.stringify({id})});
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
