# WSL Setup: Installing Go

This guide explains how to install the Go programming language inside WSL
(Windows Subsystem for Linux) so you can build and run `casdoor-aiguard`.

This project requires **Go 1.25.0 or newer** (see `go.mod`).

## 1. Open your WSL terminal

Launch your WSL distribution (for example, Ubuntu) from the Start menu, or run
`wsl` from PowerShell / Command Prompt.

## 2. Remove any old Go installation (optional)

If an older Go version is already installed, remove it first to avoid conflicts:

```bash
sudo rm -rf /usr/local/go
```

> Note: Do **not** install Go with `apt install golang-go`. The version in the
> distro package repositories is usually too old for this project.

## 3. Download the official Go release

Download the latest Linux tarball from the official Go website. For a 64-bit
(amd64) system:

```bash
cd /tmp
curl -LO https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
```

If you are on an ARM-based machine, use `go1.25.0.linux-arm64.tar.gz` instead.
Check <https://go.dev/dl/> for the newest version.

## 4. Extract Go to /usr/local

```bash
sudo tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz
```

This creates `/usr/local/go`.

## 5. Add Go to your PATH

Add Go's `bin` directory to your `PATH` by appending the following line to your
shell profile (`~/.bashrc` for bash, or `~/.zshrc` for zsh):

```bash
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
```

Then reload the profile so the change takes effect in the current session:

```bash
source ~/.bashrc
```

## 6. Verify the installation

```bash
go version
```

You should see output similar to:

```
go version go1.25.0 linux/amd64
```

## 7. Build and run the project

From the project directory inside WSL:

```bash
go mod download   # download dependencies
go build ./...    # compile the project
go run .          # run the application
```

## Tips

- **Work inside the Linux filesystem.** For best performance, clone and build
  the project under your WSL home directory (e.g. `~/casdoor-aiguard`) rather
  than under `/mnt/c/...`. Building on the Windows-mounted filesystem is
  significantly slower.
- **Module cache and binaries** live in `~/go` by default. You can optionally
  add `~/go/bin` to your `PATH` to run installed Go tools:

  ```bash
  echo 'export PATH=$PATH:$HOME/go/bin' >> ~/.bashrc
  source ~/.bashrc
  ```
- **Upgrading Go later:** repeat steps 2–4 with the newer version tarball.

## Running as root (for the transparent proxy)

aiguard's automatic transparent redirect installs `iptables` rules on startup,
which requires **root** (`euid 0`). If you run it as a normal user you'll see:

```
transparent redirect not enabled (root privileges required to manage iptables).
Running as an explicit forward proxy instead...
```

That's a safe fallback — aiguard still works as an explicit forward proxy
(point agents at `HTTP_PROXY`/`HTTPS_PROXY=http://127.0.0.1:9090`). But to get
the fully transparent redirect, the process must run as root.

### From a WSL terminal

Just launch it with `sudo`:

```bash
sudo ./aiguard        # or: sudo $(which go) run .
```

### From GoLand (debugging)

GoLand launches the program in WSL as your default user and has no "run as
root" option, so make **root the WSL default user** for that distro. Add a
`[user]` block to `/etc/wsl.conf`:

```bash
sudo tee /etc/wsl.conf >/dev/null <<'EOF'
[boot]
systemd=true

[user]
default=root
EOF
```

> If `/etc/wsl.conf` already has a `[boot]` section, keep it and just add the
> `[user]` block.

Then, from Windows (PowerShell or Command Prompt), restart WSL:

```
wsl --shutdown
```

Reopen WSL / restart GoLand and confirm:

```bash
whoami        # -> root
```

Now GoLand's Run/Debug configuration runs as root, and you'll see
`transparent redirect enabled` at startup instead of the warning.

> **Why not `setcap`?** Granting `CAP_NET_ADMIN` to the binary doesn't work here
> for two reasons: GoLand builds to `/mnt/c/...` (drvfs), which doesn't support
> Linux capabilities/xattrs, and aiguard's check requires a real root euid.
> Running as root (above) is the reliable path on a dev WSL box.

> **Note:** making root the default user affects the *entire* WSL distro — fine
> for a dedicated dev distro, but avoid it on a shared machine. On a real Linux
> host you'd instead run `sudo ./aiguard` or a systemd unit as root.
