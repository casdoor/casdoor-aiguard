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
page. This hook is installed and removed by AIGuard's agent **Patch** button -
do not edit it by hand, and do not remove it manually: use **Unpatch**, which
also restores the config entry this hook needed.

## What It Does

1. Subscribes to the command, message, session and gateway event streams
2. Summarizes each event, dropping the gateway config and bootstrap file bodies
   so provider API keys and whole documents never leave the agent
3. Posts the result to AIGuard's `/api/records` endpoint

## Failure Behavior

The post is fire-and-forget with a 3 second timeout and every error is
swallowed, so an AIGuard that is stopped, unreachable or slow cannot block or
break the agent.

## Configuration

The AIGuard endpoint is written into `handler.js` when the hook is installed.
Set `AIGUARD_RECORDS_URL` in the environment to override it - useful when the
agent runs in a container or WSL and has to reach the host by another address.
