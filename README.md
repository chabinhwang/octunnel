# octunnel

Expose your local [OpenCode](https://opencode.ai) server to the internet via [Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/) — in one command.

## Install

```bash
# Quick install (macOS / Linux)
curl -fsSL https://raw.githubusercontent.com/chabinhwang/octunnel/main/install.sh | bash

# Homebrew
brew install chabinhwang/tap/octunnel

# Go
go install github.com/chabinhwang/octunnel@latest
```

### Platform Support

| Platform | Status |
|----------|--------|
| macOS | Fully supported |
| Linux | Fully supported |
| Windows | Not yet supported (requires Unix syscalls) |

### Prerequisites

<details>
<summary><strong>macOS</strong></summary>

```bash
npm install -g opencode
brew install cloudflared
# lsof is pre-installed
```
</details>

<details>
<summary><strong>Linux (Debian/Ubuntu)</strong></summary>

```bash
npm install -g opencode

# cloudflared
curl -fsSL https://pkg.cloudflare.com/cloudflare-main.gpg \
  | sudo tee /usr/share/keyrings/cloudflare-main.gpg >/dev/null
echo "deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared $(lsb_release -cs) main" \
  | sudo tee /etc/apt/sources.list.d/cloudflared.list
sudo apt update && sudo apt install cloudflared

# clipboard (optional, for URL auto-copy)
sudo apt install xclip   # or xsel

# lsof is pre-installed on most distros
```
</details>

<details>
<summary><strong>Linux (Arch)</strong></summary>

```bash
npm install -g opencode
pacman -S cloudflared xclip
# lsof is pre-installed
```
</details>

<details>
<summary><strong>Windows</strong> (not yet supported)</summary>

Windows is not supported in the current version due to Unix-specific process management (`Setpgid`, `lsof`, signal handling). Contributions welcome.

If you want to prepare the prerequisites for future support:

```powershell
npm install -g opencode
winget install Cloudflare.cloudflared   # or: choco install cloudflared
# clipboard: built-in 'clip' command
# port detection: netstat -ano | findstr LISTENING
```
</details>

## Quick Start

### One-command public URL (Quick Tunnel)

```bash
octunnel
```

This will:
1. Start `opencode serve`
2. Detect the local port
3. Open a Cloudflare Quick Tunnel (`*.trycloudflare.com`)
4. Copy the public URL to your clipboard
5. Display a QR code in the terminal

No login or configuration needed.

### Fixed domain (Named Tunnel)

Each command prints the **next step** to guide you through the setup flow:

```bash
# 1. Login to Cloudflare (opens browser)
octunnel login

# 2. Create tunnel + connect DNS
octunnel auth

# 3. Run with your fixed domain
octunnel run
```

### Switch domain

```bash
octunnel switch domain
```

Re-login to a different Cloudflare domain and update DNS routing.

## Commands

| Command | Description |
|---------|-------------|
| `octunnel` | Quick Tunnel — instant public URL, no auth needed |
| `octunnel login` | Login to Cloudflare + set base domain |
| `octunnel auth` | Create Named Tunnel + DNS route |
| `octunnel run` | Run Named Tunnel with fixed domain |
| `octunnel switch domain` | Change to a different domain |
| `octunnel reset` | Reset config and delete Cloudflare tunnel (CNAME must be removed manually) |
| `octunnel remove` | Completely uninstall octunnel data and delete tunnel |

## State & Configuration

All state is stored in `~/.octunnel/config.json` (atomic writes with `.bak` backup).

```json
{
  "certPemPath": "/Users/me/.cloudflared/cert.pem",
  "baseDomain": "example.com",
  "tunnelId": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "tunnelName": "octunnel",
  "credentialsFilePath": "/Users/me/.cloudflared/xxxxxxxx.json",
  "hostname": "open.example.com",
  "operationStatus": "completed",
  "currentPhase": "hostname_saved",
  "mode": "named"
}
```

The cloudflared tunnel config is written to a **separate file** at `~/.octunnel/cloudflared.yml` — octunnel **never** touches `~/.cloudflared/config.yml`. This file is passed via the `--config` flag when running `cloudflared tunnel run`.

## Recovery & Fault Tolerance

octunnel is designed to handle interruptions gracefully:

| Scenario | Behavior |
|----------|----------|
| Ctrl+C during `octunnel` | Processes cleaned up, state saved as "interrupted" |
| Ctrl+C during `octunnel login` | Cert deleted, forces fresh login on next run |
| Tunnel created but DNS not routed | `octunnel auth` resumes from DNS step |
| `opencode` alive but `cloudflared` died | Reuses opencode, restarts cloudflared only |
| `cloudflared` alive but `opencode` died | Kills orphan cloudflared, starts fresh |
| Both processes alive from previous run | Verifies process names (not just PIDs), reuses both, displays existing URL |
| Config file corrupted | Restores from `.bak` backup |
| `cloudflared.yml` write failure | Atomic write (tmp + rename) prevents partial writes |
| Duplicate tunnel name | Auto-retries with `octunnel1`, `octunnel2`, ... |
| Stale lock file | Auto-cleaned only when PID is confirmed dead; if PID is alive (even a different process), user must manually remove the lock file |

### Login Recovery Policy (Conservative)

The login command uses a **conservative** recovery policy:
- If login state is even slightly unclear (cert exists but no domain, or vice versa), the existing `cert.pem` is **deleted** and a fresh login is performed.
- `octunnel login` always performs a fresh `cloudflared tunnel login` — it never silently reuses an existing cert.
- Only a fully complete state (cert + domain both valid) is considered "done".

## Lock File

`~/.octunnel/octunnel.lock` prevents concurrent octunnel instances. Stale locks are auto-cleaned only when the recorded PID is confirmed dead. If the PID is still alive (even if it belongs to a different process), the lock is **not** removed automatically — you must delete it manually.

## Uninstall

```bash
# 1. Remove config, tunnel, and all octunnel data
octunnel remove

# 2. Remove the binary
brew uninstall octunnel          # if installed via Homebrew
# or
rm $(which octunnel)             # if installed via curl or go install
```

> **Note:** `octunnel remove` deletes the Cloudflare tunnel but does NOT delete CNAME DNS records. Remove them manually from the [Cloudflare dashboard](https://dash.cloudflare.com).

## Known Limitations

- **Windows not supported** — Unix-specific syscalls (`Setpgid`, `lsof`, POSIX signals) are required
- Quick Tunnel URLs are temporary and change every run
- `cloudflared tunnel login` requires a browser — headless environments need manual cert setup
- Port detection relies on `lsof` output parsing (macOS/Linux)
- Clipboard: `pbcopy` (macOS), `xclip`/`xsel` (Linux) — Windows `clip` not yet wired
- Process detection uses `ps` and `pgrep` (pre-installed on macOS/Linux)
- The tool does **not** use the Cloudflare API — all operations go through `cloudflared` CLI

## License

MIT
