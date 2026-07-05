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

## 5. Verify the installation

```bash
openclaw --version         # show the installed version
openclaw doctor            # check the environment for problems
openclaw gateway status    # confirm the background gateway is running
```

## Tips

- **Work inside the Linux filesystem.** For best performance, keep OpenClaw's
  data under your WSL home directory (e.g. `~/`) rather than under `/mnt/c/...`.
- **Upgrading later:** re-run the install script, or `npm install -g
  openclaw@latest`, then restart the daemon with
  `openclaw gateway restart` (or repeat `openclaw onboard --install-daemon`).
- **Accessing the web UI from Windows:** WSL2 forwards `localhost`, so you can
  usually open the OpenClaw UI in your Windows browser at the address shown
  during onboarding (for example `http://localhost:<port>`).
- **Package managers:** `pnpm` and `bun` are also supported. With pnpm you must
  run `pnpm approve-builds -g` after installing, since it requires explicit
  approval for packages with build scripts.

## References

- Official install guide: <https://docs.openclaw.ai/install>
- Project homepage: <https://openclaw.ai/>
- Source code: <https://github.com/openclaw/openclaw>
