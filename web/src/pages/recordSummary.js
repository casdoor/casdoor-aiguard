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

const inputContainerKeys = ["tool_input", "toolInput", "input", "arguments", "parameters", "params"];

const commonKeyPriority = [
  "command", "cmd", "script",
  "url", "uri", "href",
  "query", "search_query", "searchQuery", "search_term", "searchTerm",
  "file_path", "filePath", "notebook_path", "notebookPath", "path", "filename",
  "pattern", "glob",
  "task_id", "taskId", "skill",
  "subject", "prompt", "description", "action", "name", "id",
];

// Keys the fallback must never present as a call's key input. Three kinds end
// up here, and the list is normalized so spelling variants collapse:
//   - the envelope auditutil.EncodeBoundedJSON writes around an oversized
//     payload (truncated, originalBytes, preview),
//   - hook metadata that describes the report rather than the operation
//     (session_id, tool_name, hook_event_name, transcript_path, cwd, model...),
//   - bulk operands already surfaced elsewhere or redacted by the collector
//     (content, old_string, new_string, tool_response).
// A collector that grows a new metadata field only costs a duplicate-looking
// column until its name is added here, so the list stays best-effort.
const ignoredFallbackKeys = new Set([
  "agenteffort", "content", "cwd", "durationms", "effort", "hookeventname", "model",
  "newstring", "oldstring", "originalbytes", "permissionmode", "preview", "promptid",
  "sessionid", "toolname", "toolresponse", "tooluseid", "transcriptpath", "truncated",
]);

// Mirrors auditutil.SensitiveKey in auditutil/sanitize.go, which already
// redacted these values before they were stored. This copy is a second line of
// defence for a collector that bypassed the Go path, so it is deliberately a
// superset: an extra name here only costs one skipped column.
const sensitiveKeyPattern = /password|passwd|secret|token|credential|authorization|cookie|privatekey|apikey|accesskey/;

// Two hints that order what a generic MCP tool leaves for the fallback: names
// that usually carry what a call is about, and names that merely qualify it.
// Neither list needs to be complete - anything unlisted ranks between them.
const preferredFallbackKeys = new Set([
  "text", "message", "body", "input", "value", "expression", "code", "sql",
  "statement", "title", "summary", "note", "comment", "reason", "goal",
  "instruction", "instructions",
]);

const deprioritizedFallbackKeys = new Set([
  "type", "kind", "mode", "format", "channel", "target", "scope", "owner",
  "repo", "branch", "limit", "offset", "count", "page", "size", "timeout",
  "locale", "language", "version", "enabled", "force", "dryrun", "recursive",
  "verbose",
]);

function normalizeKey(value) {
  return String(value || "").toLowerCase().replace(/[^a-z0-9]/g, "");
}

function objectValue(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : null;
}

function parseObject(value) {
  let parsed = value;
  for (let attempt = 0; attempt < 2; attempt += 1) {
    if (typeof parsed !== "string") {
      break;
    }
    try {
      parsed = JSON.parse(parsed);
    } catch {
      return null;
    }
  }
  return objectValue(parsed);
}

function inputObject(payload) {
  let containerKey = "";
  let input = null;
  for (const key of inputContainerKeys) {
    const entry = parseObject(payload[key]);
    if (entry) {
      containerKey = key;
      input = entry;
      break;
    }
  }
  if (!input) {
    return {input: payload, wrapped: false};
  }

  // JSON-RPC keeps actual tool arguments one level below params. Once a real
  // input/arguments envelope is reached, stop: an arbitrary tool may itself
  // have a legitimate field named "input" alongside its other parameters.
  if (containerKey === "params") {
    for (const key of inputContainerKeys.filter((candidate) => candidate !== "params")) {
      const entry = parseObject(input[key]);
      if (entry) {
        input = entry;
        break;
      }
    }
  }
  return {input, wrapped: true};
}

function displayValue(value) {
  if (typeof value === "string") {
    return value.trim();
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  if (Array.isArray(value)) {
    if (value.length === 0) {
      return "";
    }
    if (value.every((item) => ["string", "number", "boolean"].includes(typeof item))) {
      return value.join(", ");
    }
    return `${value.length} item${value.length === 1 ? "" : "s"}`;
  }
  if (objectValue(value)) {
    const count = Object.keys(value).length;
    return count ? `${count} field${count === 1 ? "" : "s"}` : "";
  }
  return "";
}

function labelFor(key) {
  const normalized = normalizeKey(key);
  if (["command", "cmd", "script"].includes(normalized)) {
    return "Command";
  }
  if (["url", "uri", "href"].includes(normalized)) {
    return "URL";
  }
  if (["query", "searchquery", "searchterm"].includes(normalized)) {
    return "Query";
  }
  if (["filepath", "notebookpath", "path", "filename"].includes(normalized)) {
    return "Path";
  }
  if (["pattern", "glob"].includes(normalized)) {
    return "Pattern";
  }
  if (normalized === "id") {
    return "ID";
  }
  const label = String(key)
    .replace(/[_-]+/g, " ")
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/^./, (letter) => letter.toUpperCase());
  return label.replace(/\bid\b/gi, "ID");
}

function findField(input, keys) {
  const entries = Object.entries(input);
  for (const candidate of keys) {
    const normalizedCandidate = normalizeKey(candidate);
    const match = entries.find(([key]) => normalizeKey(key) === normalizedCandidate);
    if (!match) {
      continue;
    }
    const value = displayValue(match[1]);
    if (value) {
      return {label: labelFor(match[0]), value};
    }
  }
  return null;
}

function toolKeyPriority(record) {
  const tool = normalizeKey(`${record.toolName || ""} ${record.mcpTool || ""}`);
  if (/(bash|shell|terminal|exec|command)/.test(tool)) {
    return ["command", "cmd", "script"];
  }
  if (/(fetch|http|url|browser|navigate)/.test(tool)) {
    return ["url", "uri", "href"];
  }
  if (/(search|grep|find|glob)/.test(tool)) {
    return ["query", "search_query", "searchQuery", "search_term", "searchTerm", "pattern"];
  }
  if (/(read|write|edit|file|notebook)/.test(tool)) {
    return ["file_path", "filePath", "notebook_path", "notebookPath", "path", "filename"];
  }
  if (/(task|agent|prompt)/.test(tool)) {
    return ["subject", "description", "prompt", "task_id", "taskId"];
  }
  return [];
}

function structuredToolField(record, input) {
  const tool = normalizeKey(`${record.toolName || ""} ${record.mcpTool || ""}`);
  if (!/(askuser|question)/.test(tool)) {
    return null;
  }
  const questionsEntry = Object.entries(input).find(([key]) => normalizeKey(key) === "questions");
  const questions = questionsEntry && questionsEntry[1];
  if (!Array.isArray(questions) || questions.length === 0) {
    return null;
  }
  const first = objectValue(questions[0]);
  if (!first) {
    return null;
  }
  const question = Object.entries(first).find(([key]) => normalizeKey(key) === "question");
  const value = question && displayValue(question[1]);
  return value ? {label: "Question", value} : null;
}

// displayValue turns a nested operand into a shape count ("3 fields") rather
// than its contents, which identifies a call far worse than any scalar beside
// it does.
function isShapeSummary(rawValue) {
  if (Array.isArray(rawValue)) {
    return !rawValue.every((item) => ["string", "number", "boolean"].includes(typeof item));
  }
  return Boolean(objectValue(rawValue));
}

function fallbackRank(normalizedKey, rawValue) {
  if (isShapeSummary(rawValue)) {
    return 3;
  }
  if (preferredFallbackKeys.has(normalizedKey)) {
    return 0;
  }
  if (deprioritizedFallbackKeys.has(normalizedKey)) {
    return 2;
  }
  return 1;
}

// The last resort for a tool whose schema aiguard cannot know. Taking whatever
// field happens to come first in the payload is arbitrary - a Slack call would
// be summarized by its channel rather than the message sent - so candidates are
// ranked and ties fall back to payload order, which keeps the pick stable for a
// given record.
function fallbackField(input) {
  let best = null;
  let bestRank = Infinity;
  for (const [key, rawValue] of Object.entries(input)) {
    const normalized = normalizeKey(key);
    if (ignoredFallbackKeys.has(normalized) || sensitiveKeyPattern.test(normalized)) {
      continue;
    }
    const value = displayValue(rawValue);
    if (!value) {
      continue;
    }
    const rank = fallbackRank(normalized, rawValue);
    if (rank < bestRank) {
      best = {label: labelFor(key), value};
      bestRank = rank;
      if (rank === 0) {
        break;
      }
    }
  }
  return best;
}

// Returns the one input that best identifies what a tool call actually did.
// Collectors keep different envelopes (tool_input, input and arguments), so
// this stays deliberately defensive and falls back cleanly for arbitrary MCP
// tools whose schemas aiguard cannot know in advance.
export function recordKeyInput(record) {
  const isOperation = record && (
    record.toolName || record.mcpTool || ["tool", "mcp"].includes(record.eventType) ||
    record.intent === "mcp.tool_call"
  );
  if (!isOperation) {
    return null;
  }
  const payload = parseObject(record.object);
  if (!payload || payload.truncated === true) {
    return null;
  }
  const {input, wrapped} = inputObject(payload);
  const allowFallback = wrapped || record.mcpTool || record.eventType === "mcp" || record.intent === "mcp.tool_call";
  return structuredToolField(record, input) ||
    findField(input, [...toolKeyPriority(record), ...commonKeyPriority]) ||
    (allowFallback ? fallbackField(input) : null);
}

function toolCallKey(record) {
  if (!record.toolUseId) {
    return "";
  }
  return [record.agent, record.sessionKey, record.toolUseId].join("\u0000");
}

// recordKeyInput parses a record's whole stored payload, which is capped at
// 64 KB, and the Records page reloads every few seconds. Summarizing a full
// batch from scratch on every poll would re-parse megabytes of JSON to redraw
// the twenty rows a page actually shows, so summaries are memoized: a stored
// payload never changes afterwards - only the feedback a person writes does -
// which makes the record id a sound cache key.
const summaryCache = new Map();

function cachedKeyInput(record) {
  const id = record && record.id;
  if (!id) {
    return recordKeyInput(record);
  }
  if (summaryCache.has(id)) {
    return summaryCache.get(id);
  }
  const summary = recordKeyInput(record);
  summaryCache.set(id, summary);
  return summary;
}

// Bound the cache to the batch on screen. Filtering and paging retire ids for
// good, and this page is long-lived, so without a sweep the map would grow for
// as long as the tab stays open.
function pruneSummaryCache(liveIds) {
  if (summaryCache.size === liveIds.size) {
    return;
  }
  for (const id of summaryCache.keys()) {
    if (!liveIds.has(id)) {
      summaryCache.delete(id);
    }
  }
}

// Some collectors put input only on the attempted half of a call. Carry the
// same summary to its result row via the tool-use id, without changing or
// inventing any stored audit data.
export function recordsWithKeyInputs(records) {
  const summaries = records.map((record) => cachedKeyInput(record));
  pruneSummaryCache(new Set(records.map((record) => record.id).filter(Boolean)));
  const byToolCall = new Map();
  records.forEach((record, index) => {
    const key = toolCallKey(record);
    if (key && summaries[index]) {
      byToolCall.set(key, summaries[index]);
    }
  });
  return records.map((record, index) => ({
    ...record,
    keyInput: summaries[index] || byToolCall.get(toolCallKey(record)) || null,
  }));
}
