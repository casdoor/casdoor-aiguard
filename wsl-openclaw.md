# WSL Setup: Installing OpenClaw

This guide explains how to install [OpenClaw](https://openclaw.ai/) — a free,
open-source, self-hosted personal AI assistant (formerly *ClawdBot* /
*MoltBot*) — inside WSL (Windows Subsystem for Linux).

OpenClaw runs on Node.js and installs a background daemon (a systemd user
service on Linux/WSL2). It requires **Node.js 22.19+, 23.11+, or 24+**
(Node 24 is recommended).

## 1. Open your WSL terminal

Launch your WSL distribution (for example, Ubuntu) from the Start menu, or run
`wsl` from PowerShell / Command Prompt. These instructions assume **WSL2**.

## 2. Install Node.js (22.19+, 23.11+, or 24+)

If you do not already have a recent Node.js, install it. The `apt` package is
usually too old, so use the NodeSource repository or `nvm`.

**Option A — NodeSource (installs Node 24):**

```bash
curl -fsSL https://deb.nodesource.com/setup_24.x | sudo -E bash -
sudo apt-get install -y nodejs
```

**Option B — nvm (per-user, no sudo):**

```bash
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash
source ~/.bashrc
nvm install 24
nvm use 24
```

Verify:

```bash
node --version   # should print v24.x (or v22.19+/v23.11+)
```

## 3. Install OpenClaw

### Option A — Quick install script (recommended)

This one-liner detects your OS, installs Node.js if needed, installs OpenClaw,
and starts onboarding:

```bash
curl -fsSL https://openclaw.ai/install.sh | bash
```

### Option B — Install with npm

```bash
npm install -g openclaw@latest
```

> If you used **nvm** in step 2, no `sudo` is needed. If Node was installed
> system-wide (NodeSource) and you get an `EACCES` permission error, either
> prefix with `sudo` or configure an npm global prefix in your home directory.

## 4. Run onboarding and install the daemon

```bash
openclaw onboard --install-daemon
```

On Linux/WSL2 this registers OpenClaw as a **systemd user service** so it keeps
running in the background. Follow the onboarding prompts to configure your AI
provider and settings.

> **Note:** systemd user services require systemd to be enabled in WSL. On
> recent WSL versions it is on by default. If not, add the following to
> `/etc/wsl.conf` and then run `wsl --shutdown` from Windows to restart:
>
> ```ini
> [boot]
> systemd=true
> ```

## 5. Configure a model provider (API key)

The gateway can run, but the agent needs credentials for an LLM provider before
it can answer. Without this you will hit:

```
No API key found for provider "openai" ... | missing-provider-auth
```

Check which providers have auth:

```bash
openclaw models status     # look for "effective=missing" / "Missing auth"
```

The default model is `openai/gpt-5.5`, so you need an OpenAI API key. Add it
**interactively** so the key is not stored in your shell history — the command
prompts for the key and writes it to
`~/.openclaw/agents/main/agent/auth-profiles.json`:

```bash
openclaw models auth paste-api-key --provider openai
# paste your sk-... key when prompted
```

> **Security:** do **not** pass the key as a command-line argument (e.g.
> `... paste-api-key <key>`). It would be saved to your shell history and is
> easy to leak. Always let the command prompt for it. If a key is ever exposed,
> revoke it at the provider and generate a new one.

Then restart the gateway so the agent reloads the credentials:

```bash
systemctl --user restart openclaw-gateway.service
openclaw models status     # openai should no longer be "missing"
```

### Example: using DeepSeek instead of OpenAI

DeepSeek uses an OpenAI-compatible API (`deepseek/deepseek-chat` for the general
V3 model, `deepseek/deepseek-reasoner` for the R1 reasoning model). Switching to
it takes three parts: **(1)** add the key, **(2)** register the provider's
models in config, and **(3)** set it as the default model, then restart.

**1. Add the API key** (interactive; if the plain prompt closes without letting
you type, pipe the key via stdin so it stays out of your shell history):

```bash
read -rs DSKEY    # paste your DeepSeek key at the (hidden) prompt, then Enter
printf '%s' "$DSKEY" | openclaw models auth paste-api-key --provider deepseek
unset DSKEY
```

**2. Register the provider models.** DeepSeek is in the model *catalog*, but the
runtime still needs `models.providers.deepseek` defined in config, otherwise you
get `Unknown model: deepseek/deepseek-chat ... no matching
models.providers["deepseek"].models[] entry`. Register it with the
OpenAI-compatible adapter and endpoint. Save this as `deepseek.patch.json5`:

```json5
{
  models: {
    providers: {
      deepseek: {
        api: "openai-completions",
        baseUrl: "https://api.deepseek.com",
        models: [
          { id: "deepseek-chat", name: "deepseek-chat" },
          { id: "deepseek-reasoner", name: "deepseek-reasoner", reasoning: true }
        ]
      }
    }
  }
}
```

Then validate and apply it:

```bash
openclaw config patch --file deepseek.patch.json5 --dry-run   # validate
openclaw config patch --file deepseek.patch.json5             # apply
```

**3. Set the default model and restart:**

```bash
openclaw models set deepseek/deepseek-chat     # or deepseek/deepseek-reasoner
systemctl --user restart openclaw-gateway.service
openclaw models status                         # Default deepseek/..., status=usable
```

Verify end to end with a one-shot call:

```bash
openclaw infer model run --model deepseek/deepseek-chat --prompt 'Reply with exactly: OK'
```

The same pattern works for other providers — run `openclaw models list --all` to
see the catalog, add the key with `paste-api-key --provider <id>`, register the
provider under `models.providers.<id>` (with the matching `api` adapter and
`baseUrl`), then `models set <provider>/<model>`. You do not have to delete other
providers' keys; the agent uses whatever the default model points to. Keep a
previous provider as backup with `openclaw models fallbacks`.

> **Prefer an environment variable?** You can instead export the key (e.g.
> `OPENAI_API_KEY`) and enable "Shell env" in the config, but that requires
> injecting the variable into the systemd service environment and is more
> fiddly than `paste-api-key`.

## 6. Verify the installation

```bash
openclaw --version         # show the installed version
openclaw doctor            # check the environment for problems
openclaw status            # overall status (gateway, dashboard, sessions)
openclaw gateway status    # gateway service status + reachability probe
```

A healthy gateway shows `reachable` and the service as `running (state active)`.

## 7. Open the dashboard from Windows

Once the gateway is running, the dashboard is served on port **18789** by
default. WSL2 forwards `localhost` to the Windows host, so open this URL in any
Windows browser (Edge, Chrome, etc.):

```
http://127.0.0.1:18789/
```

(equivalently `http://localhost:18789/`). The exact address is printed in the
`Dashboard` row of `openclaw status`. If the page asks for a token, use the
value of `gateway.auth.token` from `~/.openclaw/openclaw.json`.

## Troubleshooting

### Gateway service fails to start (`stopped (state failed)`)

If `openclaw status` shows the gateway as `unreachable` /
`connect ECONNREFUSED 127.0.0.1:18789` and the service as `failed`, inspect the
logs:

```bash
systemctl --user status openclaw-gateway.service --no-pager
journalctl --user -u openclaw-gateway.service -n 50 --no-pager
```

A common cause right after install is **missing configuration** — the log shows
`Missing config ...` and the process exits with `status=78/CONFIG`. This happens
when onboarding did not set the gateway mode. Fix it by setting the mode
explicitly, then restart:

```bash
openclaw config set gateway.mode local
systemctl --user restart openclaw-gateway.service
openclaw gateway status     # should now report reachable
```

`openclaw doctor` will also flag `gateway.mode is unset` and suggest the same
fix.

### Agent fails with `missing-provider-auth`

If the gateway is running but the agent replies with
`No API key found for provider "openai" ... | missing-provider-auth`, no LLM
provider credentials are configured. See [step 5](#5-configure-a-model-provider-api-key):

```bash
openclaw models status                              # confirm what is missing
openclaw models auth paste-api-key --provider openai
systemctl --user restart openclaw-gateway.service
```

### Service does not survive logout / reboot

WSL stops your user's systemd instance when you log out. Enable lingering so the
gateway keeps running:

```bash
sudo loginctl enable-linger $USER
```

## Tips

- **Work inside the Linux filesystem.** For best performance, keep OpenClaw's
  data under your WSL home directory (e.g. `~/`) rather than under `/mnt/c/...`.
- **Upgrading later:** re-run the install script, or `npm install -g
  openclaw@latest`, then restart the daemon with
  `openclaw gateway restart` (or repeat `openclaw onboard --install-daemon`).
- **Package managers:** `pnpm` and `bun` are also supported. With pnpm you must
  run `pnpm approve-builds -g` after installing, since it requires explicit
  approval for packages with build scripts.

## References

- Official install guide: <https://docs.openclaw.ai/install>
- Project homepage: <https://openclaw.ai/>
- Source code: <https://github.com/openclaw/openclaw>
