# casdoor-aiguard

**A policy enforcement point (PEP) for AI agents.** casdoor-aiguard runs on a
Linux host and transparently intercepts the outbound (egress) traffic of the AI
agents running there, extracts the *high-level intent* of each sensitive
operation (e.g. "this is a payment of $9,000 to sketchy-llc"), and asks
[Casdoor](https://github.com/casdoor/casdoor) — acting as the policy decision
point (PDP) and OAuth authorization server — whether to **allow**, **deny**, or
**step-up** (require human approval).

> If Casdoor is the *door*, casdoor-aiguard is the officer standing at it,
> inspecting every agent one by one and deciding whether to let it through.

The defining constraint: **interception requires no changes to any agent's code
or configuration.** Agents — open-source or commercial — are never modified and
never even know aiguard is there. Enforcement happens at the host's egress
layer, below the agent.

---

## Table of contents

- [How it works](#how-it-works)
- [Requirements](#requirements)
- [Quick start](#quick-start)
- [Interception modes](#interception-modes)
- [Trusting the CA](#trusting-the-ca)
- [Configuration](#configuration)
- [Policy file](#policy-file)
- [Casdoor integration](#casdoor-integration)
- [Security defaults](#security-defaults)
- [Web UI](#web-ui)
- [HTTP API](#http-api)
- [Project layout](#project-layout)
- [Development & testing](#development--testing)
- [Roadmap](#roadmap)
- [License](#license)

---

## How it works

```
   AI agent (unmodified)
        │  outbound HTTP / HTTPS  (e.g. POST https://api.example.com/v1/payments)
        ▼
  ┌───────────────────────────────────────────────────────────────┐
  │  iptables/nftables REDIRECT  ──►  aiguard transparent proxy     │
  │                                                                 │
  │   1. terminate TLS with a leaf cert from aiguard's local CA     │
  │   2. recognizers/  extract intent from the plaintext:           │
  │        • MCP JSON-RPC  tools/call  (structured, intent is free) │
  │        • payment API   (pluggable per-API recognizer)           │
  │   3. object/policy    evaluate local rules → allow/deny/step-up/pdp
  │   4. casdoorclient    for "pdp" verdicts, ask Casdoor           │
  │   5. enforce:  allow → forward   deny → 403   step-up → (CIBA)  │
  │   6. object/store     record the event (dashboard + audit log)  │
  └───────────────────────────────────────────────────────────────┘
        │  forwarded (if allowed) to the real destination
        ▼
   api.example.com
```

The intent extraction is **per-API, not per-agent**: you write a recognizer once
for the shape of a destination API (or for a protocol like MCP), and every agent
that calls that API is covered automatically.

## Requirements

- **Linux** for the production transparent-interception path (it uses
  `iptables`/`nftables` `REDIRECT` and the kernel's `SO_ORIGINAL_DST`).
  Downloading rules and installing the CA both need **root**.
- **Go 1.25+** to build the backend.
- **Node.js + Yarn** to build the web UI.
- A reachable **Casdoor** instance if you want real PDP decisions (otherwise
  aiguard runs on local policy alone — see [Casdoor integration](#casdoor-integration)).

> For local development and testing, aiguard can also run as an **explicit
> forward proxy** on any OS (macOS/Windows included) — no root, no iptables.
> See [Interception modes](#interception-modes).

## Quick start

```bash
# 1. Build the web UI (produces web/build, served by the backend)
cd web && yarn install && yarn build && cd ..

# 2. Build and run the backend
go build -o aiguard .
./aiguard
```

On first run aiguard generates its local CA under `./certs/` and writes a
default `conf/policy.yaml`. The management UI + API is served on
`http://localhost:9000`; the interception proxy listens on `:9090`.

On Linux, **run aiguard as root and it installs the transparent redirect
itself** (and removes it on exit) — no manual iptables step:

```bash
sudo ./scripts/install_ca.sh ./certs/aiguard-ca.crt   # trust the MITM CA once
sudo ./aiguard                                         # auto-installs iptables redirect on start
# ... run your agents; press Ctrl-C to stop and restore iptables ...
```

aiguard excludes its own uid from the redirect so its forwarded/upstream
traffic never loops back into itself. Graceful shutdown (Ctrl-C, `kill`/SIGTERM,
`systemctl stop`) tears the rules down; because `SIGKILL`/crash can't be caught,
the next startup clears any leftover rules idempotently. `scripts/setup_iptables.sh`
and `scripts/cleanup_iptables.sh` remain available for manual control or if you
prefer to disable auto-management (`autoTransparentProxy = false`).

## Interception modes

aiguard picks a mode per connection automatically:

| Mode | When | How to enable | Needs root? | Agent changes? |
|------|------|---------------|-------------|----------------|
| **Transparent** (production) | connection arrived via an iptables/nftables REDIRECT (`SO_ORIGINAL_DST` resolves) | automatic on startup when run as root (`autoTransparentProxy = true`); or `scripts/setup_iptables.sh` | yes | none |
| **Explicit proxy** (dev/testing) | no redirect present | point the client at aiguard: `export HTTPS_PROXY=http://localhost:9090 HTTP_PROXY=http://localhost:9090` | no | env var only |

Both modes run the *identical* recognize → decide → enforce pipeline, so you can
validate policies on a laptop and deploy the same binary transparently on Linux.

## Trusting the CA

Because aiguard terminates TLS to read the plaintext, the agent must trust
aiguard's CA or it will see certificate errors.

- **Download** it from the Web UI (Interception page) or from `GET /api/ca-cert`,
  or find it at `./certs/aiguard-ca.crt`.
- **Install** it into a host's / base image's trust store:
  `sudo ./scripts/install_ca.sh [path/to/aiguard-ca.crt]`
  (supports Debian/Ubuntu/Alpine via `update-ca-certificates` and
  RHEL/Fedora via `update-ca-trust`). Remove with `sudo ./scripts/uninstall_ca.sh`.
- Runtimes with their own bundled trust stores need the CA pointed at them
  separately: Node.js `NODE_EXTRA_CA_CERTS`, Python/requests `REQUESTS_CA_BUNDLE`,
  Go `SSL_CERT_FILE`.

## Configuration

Backend config lives in `conf/app.conf` (Beego style; every key can be
overridden by an environment variable of the same name):

| Key | Default | Meaning |
|-----|---------|---------|
| `httpport` | `9000` | management UI + API port |
| `proxyPort` | `9090` | transparent/explicit interception proxy port |
| `autoTransparentProxy` | `true` | on Linux+root, auto-install the iptables redirect on start and remove it on exit |
| `caCertDir` | `./certs` | where the local CA cert/key are stored |
| `policyFile` | `./conf/policy.yaml` | rule file (see below) |
| `auditLogFile` | `./logs/audit.log` | append-only JSONL audit log |
| `casdoorEndpoint` | `http://localhost:8000` | Casdoor base URL |
| `casdoorClientId` / `casdoorClientSecret` | *(empty)* | aiguard's own client credentials |
| `casdoorOrganization` / `casdoorApplication` | `built-in` / `app-built-in` | Casdoor org/app |
| `pdpEnforcePath` | `/api/enforce` | Casdoor enforcement endpoint |
| `failClosedOnPdpError` | `true` | deny **recognized sensitive** ops when Casdoor is unreachable |
| `passthroughUnrecognized` | `true` | allow **unrecognized** traffic straight through |
| `stepUpDefaultAction` | `deny` | verdict a step-up resolves to while CIBA is stubbed |

All of the runtime-tunable settings are also editable from the Web UI and via
`POST /api/settings`.

## Policy file

`conf/policy.yaml` is an ordered rule list. Rules are matched top-to-bottom; the
first match wins. Each rule's `action` is one of `allow`, `deny`, `step-up`, or
`pdp` (defer to Casdoor).

```yaml
enabledRecognizers:
    - mcp
    - payment-example
destinationAllowlist: []          # hosts always allowed, skipping all checks
rules:
    - id: high-value-payment-step-up
      category: payment            # matches Intent.Category
      minAmount: 1000              # only when the amount exceeds this
      action: step-up
    - id: payment-needs-pdp
      category: payment
      action: pdp                  # ask Casdoor
defaultAction: pdp                 # for a recognized-but-unmatched intent
```

A rule may also match on `toolName` (for MCP tool calls) and `destinations`
(a host list). Edit the file directly or via the Web UI / `POST /api/policy`.

## Casdoor integration

aiguard authenticates to Casdoor with its own **client credentials**
(`casdoorClientId` / `casdoorClientSecret`), then calls the enforcement endpoint
(`pdpEnforcePath`, default `/api/enforce`) for every `pdp`-action intent. The
request carries a Casbin-style `(subject, object, action)` triple — where the
subject is the originating agent, the object is the destination, and the action
is the operation — plus the extracted intent fields (amount, recipient, …) for
richer policies. Map these to a Casdoor permission on the Casdoor side.

- If **no credentials** are configured, aiguard skips the remote call and runs on
  local policy only — convenient for a first demo.
- If Casdoor is configured but **unreachable**, `failClosedOnPdpError` decides the
  outcome for sensitive operations (deny by default).

Short-lived, single-transaction **token injection** on allow, and real **CIBA**
step-up, are scaffolded for stage 2 (see [Roadmap](#roadmap)).

## Security defaults

This is a security-sensitive enforcement point, so the defaults are deliberate
and configurable:

- **Recognized sensitive operations fail *closed*.** If aiguard understood the
  request as sensitive but can't reach the PDP, it **denies** (`failClosedOnPdpError = true`).
- **Unrecognized ordinary traffic fails *open*.** Traffic no recognizer
  identified is **passed through** untouched (`passthroughUnrecognized = true`), so
  the interception layer doesn't take down all host traffic.
- **Step-up defaults to deny** while CIBA is stubbed (`stepUpDefaultAction = deny`).

## Web UI

React + Ant Design, aligned with Casdoor's frontend conventions. Built to
`web/build` and served by the backend at `http://localhost:9000`:

- **Dashboard** — live event stream: source agent, extracted intent, decision, timestamp.
- **Policy** — edit rules, thresholds, allowlists, and which recognizers are enabled.
- **Interception** — proxy port, capture settings, and CA download.
- **Casdoor Connection** — endpoint, credentials, org/app, and a connectivity test.

For UI development with hot reload, run the dev server (craco, proxies `/api` to
the backend): `yarn --cwd web start`.

## HTTP API

| Method & path | Purpose |
|---------------|---------|
| `GET /api/events?limit=200` | most recent intercepted events, newest first |
| `GET /api/policy` | current policy |
| `POST /api/policy` | replace policy |
| `GET /api/settings` | current settings (secrets masked) |
| `POST /api/settings` | update settings |
| `GET /api/ca-cert` | download the local CA certificate (PEM) |

All responses use Casdoor's `{ "status": "ok", "msg": "", "data": ... }` envelope.

## Project layout

```
main.go                 bootstrap: settings, policy, audit, CA, proxy, web
conf/                   app.conf, policy.yaml, config helpers
object/                 domain models: policy, event store, settings, audit
recognizers/            pluggable intent recognizers (MCP, payment) + registry
casdoorclient/          PEP → Casdoor client (auth, enforce)
proxy/                  interception engine: local CA, TLS MITM, transparent
                        + explicit-proxy handlers, source-process lookup
controllers/            Beego API controllers
routers/                API routes + SPA static serving
scripts/                iptables/nftables setup+cleanup, CA install/uninstall
web/                    React + Ant Design management UI
```

## Development & testing

```bash
go vet ./...
go test ./...        # includes end-to-end tests that drive the real pipeline
                     # (recognize → policy → PDP → allow/deny) through the
                     # explicit proxy, over both plaintext and HTTPS-MITM
```

The `proxy` package tests cover every decision branch (allow, deny, fail-closed,
step-up→deny, passthrough) and run on any OS — no root or iptables needed.

## Roadmap

| Stage | Scope | Status |
|-------|-------|--------|
| **1 — MVP** | user-space transparent proxy, local-CA MITM, MCP/HTTP recognition, example payment recognizer, Casdoor allow/deny, Web UI | **done** |
| **2** | real CIBA step-up, single-use token injection on allow, more recognizers, source identity enrichment (PID/cgroup → SPIFFE → Casdoor agent identity) | scaffolded |
| **3** | eBPF (sockops redirect / uprobe TLS plaintext), unbypassable kernel enforcement, cross-platform abstraction (Windows WFP) | planned |

Currently stubbed: CIBA step-up (resolves to `stepUpDefaultAction`), token
injection, SPIFFE identity, eBPF.

## License

[Apache License 2.0](LICENSE), consistent with Casdoor.