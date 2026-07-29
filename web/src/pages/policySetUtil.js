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

// Shared between the Policy Hub grid and a single policy set's page.

import React, {useState} from "react";

export const STRICTNESS_ORDER = ["strict", "moderate", "permissive"];

// The agents a policy set can be written for, in the order the filter lists
// them: Anthropic's first, then the rest roughly by how long they have been
// around. A set names exactly one of these, because its rules spell out the
// agent's own ID as the Casbin subject.
export const AGENT_ORDER = [
  "Claude Code",
  "Claude Desktop",
  "OpenClaw",
  "Codex",
  "Codex CLI",
  "Hermes Agent",
  "Cursor",
  "GitHub Copilot",
  "opencode",
  "Gemini CLI",
  "Google Antigravity",
  "Windsurf",
  "Qwen Code",
  "Trae",
  "CodeBuddy",
];

// The official site each agent belongs to, keyed by the exact display name a
// policy set stores in its "agent" field. agentIconUrl turns one of these into
// the agent's own brand mark - its site favicon - so a card advertises who it
// guards with that vendor's logo rather than a generic emoji. An agent with no
// entry here (or whose favicon fails to load) falls back to the OS emoji the
// policy set already carries.
export const AGENT_SITES = {
  "Claude Code": "claude.com",
  "Claude Desktop": "claude.ai",
  "OpenClaw": "openclaw.ai",
  "Codex": "openai.com",
  "Codex CLI": "openai.com",
  "Hermes Agent": "hermes-agent.nousresearch.com",
  "Cursor": "cursor.com",
  // Cursor's CLI has no policy set of its own yet, but the agent scan finds it
  // and an installation still deserves the brand mark of the agent it is.
  "Cursor Agent": "cursor.com",
  "GitHub Copilot": "github.com",
  "opencode": "opencode.ai",
  "Gemini CLI": "gemini.google.com",
  "Google Antigravity": "antigravity.google",
  "Windsurf": "windsurf.com",
  "Qwen Code": "qwen.ai",
  "Trae": "trae.ai",
  "CodeBuddy": "codebuddy.ai",
};

// The same agent reaches us under two spellings: a policy set carries the
// display name ("Claude Code") while the agent scan and the records carry the
// id ("claude-code"). Folding both to letters and digits alone lets one table
// answer for either spelling.
const agentKey = (agent) => String(agent || "").toLowerCase().replace(/[^a-z0-9]/g, "");

const AGENT_SITES_BY_KEY = Object.fromEntries(
  Object.entries(AGENT_SITES).map(([name, site]) => [agentKey(name), site])
);

export function agentIconUrl(agent) {
  const site = AGENT_SITES_BY_KEY[agentKey(agent)];
  return site ? `https://www.google.com/s2/favicons?domain=${site}&sz=64` : "";
}

// AgentIcon renders an agent's official brand mark for a policy set. It falls
// back to whatever the caller passes - normally the OS emoji the set carries -
// when the agent has no known site or its favicon fails to load, so a card
// never ends up with an empty icon slot.
export function AgentIcon({agent, fallback = null, size = 30}) {
  const url = agentIconUrl(agent);
  const [broken, setBroken] = useState(false);

  if (!url || broken) {
    return fallback;
  }
  return (
    <img
      src={url}
      alt={agent}
      width={size}
      height={size}
      onError={() => setBroken(true)}
      style={{display: "block", borderRadius: 6}}
    />
  );
}

// Operating systems, grouped by family. A set targets one of them - the
// package manager and the system tools it refuses differ per distribution -
// but picking a family in the filter matches every distribution under it.
export const OS_FAMILIES = [
  {name: "Windows", members: ["Windows"]},
  {name: "macOS", members: ["macOS"]},
  {name: "Linux", members: ["Ubuntu", "Debian", "CentOS", "RHEL", "Fedora", "Arch Linux", "openSUSE", "Alpine"]},
];

export function osFamily(os) {
  const family = OS_FAMILIES.find((candidate) => candidate.members.includes(os));
  return family ? family.name : os;
}

// sortByOrder keeps the known values in their canonical order and appends
// anything a hand-written policy set introduced, so a new agent still shows up
// in the filter before anyone edits this file.
export function sortByOrder(values, order) {
  const rank = (value) => {
    const index = order.indexOf(value);
    return index === -1 ? order.length : index;
  };
  return [...values].sort((a, b) => rank(a) - rank(b) || a.localeCompare(b));
}

export const STRICTNESS_COLORS = {
  strict: "red",
  moderate: "orange",
  permissive: "green",
};

// countPolicyRules ignores blank lines and comments, so a card advertises the
// number of rules a reader would actually count.
export function countPolicyRules(policy) {
  return (policy || "")
    .split("\n")
    .filter((line) => line.trim() !== "" && !line.trim().startsWith("#"))
    .length;
}

export const monospace = "Menlo, Consolas, \"Courier New\", monospace";
