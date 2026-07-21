// aiguard behaviour recorder and interceptor for OpenClaw.
//
// Installed by casdoor-aiguard into ~/.openclaw/hooks/aiguard-recorder/ and
// removed again on unpatch. Do not edit by hand: unpatching restores whatever
// was here before, so local changes are lost.
//
// The hook subscribes to OpenClaw's internal event stream. For an event that
// carries an operation the agent is about to perform, it asks aiguard for a
// verdict and throws when aiguard denies it, so an enabled policy set actually
// blocks the operation. Every other event is posted to aiguard's /api/records
// endpoint as before. Both paths fail open: if aiguard is unreachable the
// operation proceeds, so a stopped aiguard can never break the agent.

// The three values are substituted as JSON literals when aiguard installs this
// file, so a Windows install path arrives correctly escaped.
const RECORDS_URL = process.env.AIGUARD_RECORDS_URL || __AIGUARD_RECORDS_URL__;
const ENFORCE_URL = process.env.AIGUARD_ENFORCE_URL || __AIGUARD_ENFORCE_URL__;
const AGENT_PATH = __AIGUARD_AGENT_PATH__;
const POST_TIMEOUT_MS = 3000;

// Bodies are trimmed here rather than at the server, so a huge message never
// leaves the agent in the first place.
const MAX_OBJECT_CHARS = 64 * 1024;

const truncate = (text) =>
  text.length > MAX_OBJECT_CHARS ? text.slice(0, MAX_OBJECT_CHARS) + "\n...[truncated]" : text;

// cfg carries the entire gateway configuration, including provider API keys, and
// the bootstrap files are whole documents. Neither belongs in an audit record.
const SKIP_CONTEXT_KEYS = new Set(["cfg", "bootstrapFiles", "sessionEntry", "previousSessionEntry"]);

const summarizeContext = (context) => {
  const summary = {};
  for (const [key, value] of Object.entries(context || {})) {
    if (SKIP_CONTEXT_KEYS.has(key) || value === undefined || value === null) {
      continue;
    }
    summary[key] = value;
  }
  return summary;
};

// operationOf inspects an event for a tool or command the agent is about to run.
// aiguard's policy sets phrase an MCP call as "host#tool" with the intent
// "mcp.tool_call", so a recognized operation is reported the same way and can be
// matched by the very rules the Policy Hub shows. An event that carries no such
// operation (a session reset, gateway startup) returns null and is only logged.
const operationOf = (event) => {
  const context = event.context || {};
  const name =
    context.tool || context.toolName || context.tool_name || context.command || context.commandName;
  if (!name) {
    return null;
  }
  const host = context.host || context.server || "localhost";
  return {resource: `${host}#${name}`, intent: "mcp.tool_call"};
};

// post sends a record to aiguard and never waits on the answer. Used for events
// that are logged but not ruled on.
const post = async (record) => {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), POST_TIMEOUT_MS);
  try {
    await fetch(RECORDS_URL, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(record),
      signal: controller.signal,
    });
  } catch {
    // aiguard being down must never surface as an agent failure.
  } finally {
    clearTimeout(timer);
  }
};

// enforce asks aiguard for a verdict on one operation and returns true (allow)
// unless aiguard clearly answered deny. A missing or malformed answer, or a
// timeout, allows the operation: aiguard's absence must never block the agent.
const enforce = async (record) => {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), POST_TIMEOUT_MS);
  try {
    const response = await fetch(ENFORCE_URL, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(record),
      signal: controller.signal,
    });
    const body = await response.json();
    if (body && body.status === "ok" && body.data && body.data.allowed === false) {
      return {allowed: false, policySet: body.data.policySet};
    }
  } catch {
    // Unreachable or slow aiguard: fail open.
  } finally {
    clearTimeout(timer);
  }
  return {allowed: true};
};

const handler = async (event) => {
  let object = "";
  try {
    object = truncate(JSON.stringify(summarizeContext(event.context)));
  } catch {
    object = "";
  }

  const timestamp = event.timestamp instanceof Date ? event.timestamp : new Date();
  const context = event.context || {};

  const record = {
    agent: "openclaw",
    agentPath: AGENT_PATH,
    createdTime: timestamp.toISOString(),
    eventType: event.type,
    action: event.action,
    sessionKey: event.sessionKey || context.sessionKey || "",
    user: context.senderId || context.from || context.to || "",
    channel: context.channelId || context.commandSource || "",
    object,
  };

  const operation = operationOf(event);
  if (operation) {
    // Wait for the verdict: this is the point that blocks the agent. A deny
    // throws so OpenClaw aborts the operation rather than performing it.
    const decision = await enforce({...record, resource: operation.resource, intent: operation.intent});
    if (!decision.allowed) {
      throw new Error(
        `Casdoor AIGuard blocked "${operation.resource}" (policy set: ${decision.policySet || "unknown"}).`,
      );
    }
    return;
  }

  // Fire and forget for everything aiguard only logs: hooks run inside command
  // processing, so awaiting the post would add aiguard's latency to every event.
  void post(record);
};

export default handler;
