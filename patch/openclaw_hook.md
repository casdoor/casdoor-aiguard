---
name: aiguard-recorder
description: "Stream OpenClaw's behaviour log to Casdoor AIGuard"
homepage: https://github.com/casdoor/casdoor-aiguard
metadata:
  {
    "openclaw":
      {
        "emoji": "🛡️",
        "events":
          [
            "command",
            "message",
            "session:patch",
            "session:compact:before",
            "session:compact:after",
            "agent:bootstrap",
            "gateway:startup",
          ],
      },
  }
---

# AIGuard Recorder

Posts every OpenClaw event to Casdoor AIGuard, where it appears on the Records
page, and lets an enabled Policy Hub set block an operation before it runs. This
hook is installed and removed by AIGuard's agent **Patch** button - do not edit
it by hand, and do not remove it manually: use **Unpatch**, which also restores
the config entry this hook needed.

## What It Does

1. Subscribes to the command, message, session and gateway event streams
2. Summarizes each event, dropping the gateway config and bootstrap file bodies
   so provider API keys and whole documents never leave the agent
3. For an event that carries a tool or command the agent is about to run, asks
   AIGuard's `/api/enforce` endpoint for a verdict and **throws when the answer
   is deny**, so an enabled policy set aborts the operation
4. Posts every other event to AIGuard's `/api/records` endpoint

## Failure Behavior

Both paths fail open with a 3 second timeout and every error swallowed: an
AIGuard that is stopped, unreachable or slow allows the operation and cannot
break the agent. Only a clear `deny` from a reachable AIGuard blocks an
operation.

## Configuration

The AIGuard endpoint is written into `handler.js` when the hook is installed.
Set `AIGUARD_RECORDS_URL` in the environment to override it - useful when the
agent runs in a container or WSL and has to reach the host by another address.
