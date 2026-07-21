// aiguard behaviour recorder for OpenClaw.
//
// Installed by casdoor-aiguard into ~/.openclaw/hooks/aiguard-recorder/ and
// removed again on unpatch. Do not edit by hand: unpatching restores whatever
// was here before, so local changes are lost.
//
// The hook subscribes to OpenClaw's internal event stream and posts every event
// to aiguard's /api/records endpoint. It never blocks the gateway: the post is
// fire-and-forget with a short timeout, and every failure is swallowed so a
// stopped aiguard cannot break the agent.

// Both values are substituted as JSON literals when aiguard installs this file,
// so a Windows install path arrives correctly escaped.
const RECORDS_URL = process.env.AIGUARD_RECORDS_URL || __AIGUARD_RECORDS_URL__;
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

const handler = async (event) => {
  let object = "";
  try {
    object = truncate(JSON.stringify(summarizeContext(event.context)));
  } catch {
    object = "";
  }

  const timestamp = event.timestamp instanceof Date ? event.timestamp : new Date();
  const context = event.context || {};

  // Fire and forget: hooks run inside command processing, so awaiting the post
  // would add aiguard's latency to every agent event.
  void post({
    agent: "openclaw",
    agentPath: AGENT_PATH,
    createdTime: timestamp.toISOString(),
    eventType: event.type,
    action: event.action,
    sessionKey: event.sessionKey || context.sessionKey || "",
    user: context.senderId || context.from || context.to || "",
    channel: context.channelId || context.commandSource || "",
    object,
  });
};

export default handler;
