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
