# Clipboard Sync

Clipboard synchronization for Linux (Wayland) and macOS.

This repository now ships two modes:

- **Relay mode (recommended):** Go client/server architecture for multi-device sync without SSH.
- **Legacy SSH mode:** Existing Linux ↔ macOS shell script flow kept for backward compatibility.

## Relay Mode

Relay mode replaces direct SSH clipboard access with:

- a **Go relay server** that accepts multiple devices
- a **Go client** that runs on each device
- shared metadata (`device_id`, `room`, `hash`, `timestamp`, `kind`) to prevent loops and stale updates

### Features

- Multi-device fan-out through one relay service
- Room isolation and shared-token authentication
- Optional TLS on the relay socket
- Linux and macOS client support
- Text and image clipboard sync

### Relay architecture

```text
Device A client ─┐
Device B client ─┼──> Go relay server ─── broadcast to other devices in same room
Device C client ─┘
```

### Build

```bash
go build ./cmd/clipboard-sync-server
go build ./cmd/clipboard-sync-client
```

### Run the relay server

Plain TCP:

```bash
./clipboard-sync-server -listen :8484 -token your-shared-token
```

TLS:

```bash
./clipboard-sync-server \
  -listen :8484 \
  -token your-shared-token \
  -tls-cert /path/to/fullchain.pem \
  -tls-key /path/to/privkey.pem
```

### Client configuration

Copy `/home/runner/work/clipboard-sync/clipboard-sync/config/client.ini.template` to `~/.config/clipboard-sync/client.ini` and edit it.

```ini
[server]
address = relay.example.com:8484
token = REPLACE_WITH_SHARED_TOKEN
room = personal

[device]
id = linux-laptop
name = Linux Laptop

[sync]
poll_interval = 1
enable_text = true
enable_image = true

[clipboard]
image_helper_path = /Users/YOUR_USERNAME/scripts/imagecopy

[tls]
enabled = true
cert_file =
skip_verify = false
```

### Run a client

Linux:

```bash
./clipboard-sync-client -config ~/.config/clipboard-sync/client.ini
```

macOS:

```bash
./clipboard-sync-client -config ~/.config/clipboard-sync/client.ini
```

#### Client requirements

Linux:
- Wayland compositor
- `wl-paste`
- `wl-copy`

macOS:
- `pbcopy`
- `pbpaste`
- `imagecopy` helper for image sync

### Security notes

- Use TLS in any non-local deployment.
- Use a strong shared token.
- Use separate `room` values for different users or device groups.
- Prefer a private CA bundle or publicly trusted certificate instead of `skip_verify = true`.

## Legacy SSH Mode

The original Linux → SSH → macOS flow is still available.

### Features

- **Text sync**: Copy text on Linux → available on macOS, and vice versa
- **Image sync**: Copy images on Linux → available on macOS, and vice versa
- **Hash-based detection**: Only syncs when content changes
- **Notifications**: Optional desktop notifications on both systems
- **Systemd integration**: Runs as a user service

## Requirements

### Linux
- Wayland compositor (wl-paste, wl-copy)
- SSH client
- `notify-send` (libnotify)
- `iconv`, `sha256sum` (usually pre-installed)

### macOS
- SSH server enabled
- `pbcopy`/`pbpaste` (built-in)
- [imagecopy](#macos-setup) helper (auto-installed)

## Quick Start

### Linux

```bash
curl -sSL https://tiamed.github.io/clipboard-sync/install-linux.sh | bash
```

Then edit the config:
```bash
nano ~/.config/clipboard-sync/config.ini
```

Start the service:
```bash
systemctl --user enable --now clipboard-sync
```

### macOS

```bash
curl -sSL https://tiamed.github.io/clipboard-sync/install-macos.sh | bash
```

## Installation

### Linux

**One-liner (recommended):**
```bash
curl -sSL https://tiamed.github.io/clipboard-sync/install-linux.sh | bash
```

**Install dependencies manually:**

Ubuntu/Debian:
```bash
sudo apt install wl-clipboard libnotify-bin
```

Arch Linux:
```bash
sudo pacman -S wl-clipboard libnotify
```

Fedora:
```bash
sudo dnf install wl-clipboard libnotify
```

**Files installed:**
- `~/.local/bin/clipboard-sync` - Main executable
- `~/.config/clipboard-sync/config.ini` - Configuration
- `~/.local/state/clipboard-sync/` - State files
- `~/.config/systemd/user/clipboard-sync.service` - Systemd service

### Configuration

Edit `~/.config/clipboard-sync/config.ini`:

```ini
[remote]
user = your_mac_username
host = 192.168.1.100
image_copy_path = /Users/your_username/scripts/imagecopy

[sync]
min_length = 1
poll_interval = 1
enable_text = true
enable_image = true

[notifications]
enabled = true

[ssh]
options = -o BatchMode=yes -o StrictHostKeyChecking=no -o ConnectTimeout=10
```

### SSH Setup

Ensure passwordless SSH works to your Mac:

```bash
ssh-copy-id your_mac_username@your_mac_host
ssh your_mac_username@your_mac_host "echo OK"
```

### Start Service

```bash
systemctl --user enable --now clipboard-sync
systemctl --user status clipboard-sync
```

## macOS Setup

The `imagecopy` helper is required for image sync. Install it with one command:

```bash
curl -sSL https://tiamed.github.io/clipboard-sync/install-macos.sh | bash
```

This downloads a pre-compiled binary for your architecture (Intel or Apple Silicon).

**Manual installation:**

```bash
mkdir -p ~/scripts
curl -Lo ~/scripts/imagecopy https://github.com/tiamed/clipboard-sync/releases/latest/download/imagecopy-$(uname -m)
chmod +x ~/scripts/imagecopy
```

**From source:**

```bash
mkdir -p ~/scripts
swiftc macos/imagecopy.swift -o ~/scripts/imagecopy
chmod +x ~/scripts/imagecopy
```

**Usage:**
```bash
~/scripts/imagecopy /path/to/image.png     # Push image to clipboard
~/scripts/imagecopy -o /path/to/output.png # Save clipboard image to file
```

## Configuration Reference

| Section | Option | Default | Description |
|---------|--------|---------|-------------|
| `[remote]` | `user` | (required) | macOS username |
| `[remote]` | `host` | (required) | macOS hostname/IP |
| `[remote]` | `image_copy_path` | (required) | Path to imagecopy helper |
| `[sync]` | `min_length` | `1` | Minimum text length to sync |
| `[sync]` | `poll_interval` | `1` | Seconds between polls |
| `[sync]` | `enable_text` | `true` | Enable text sync |
| `[sync]` | `enable_image` | `true` | Enable image sync |
| `[notifications]` | `enabled` | `true` | Show desktop notifications |
| `[ssh]` | `options` | (see example) | SSH connection options |

## Usage

### One-Shot Commands

Use these to immediately sync without waiting for the next polling cycle. Ideal for keyboard shortcuts or window manager hooks.

```bash
# Push Linux clipboard to macOS (auto-detects text or image)
clipboard-sync sync-to

# Pull macOS clipboard to Linux (tries image first, falls back to text)
clipboard-sync sync-from

# Show help
clipboard-sync --help
```

**Keyboard shortcut examples (sway/i3/hyprland):**
```
bindsym $mod+Shift+c exec clipboard-sync sync-to
bindsym $mod+Shift+v exec clipboard-sync sync-from
```

### Start/Stop Service

```bash
systemctl --user start clipboard-sync
systemctl --user stop clipboard-sync
systemctl --user restart clipboard-sync
```

### View Logs

```bash
journalctl --user -u clipboard-sync -f
```

### Continuous Daemon

```bash
# Run without arguments for continuous polling (default)
~/.local/bin/clipboard-sync
```

## Uninstall

```bash
./scripts/uninstall.sh           # Keep config
./scripts/uninstall.sh --purge   # Remove everything
```

## Troubleshooting

### Clipboard not syncing

1. Check service status:
   ```bash
   systemctl --user status clipboard-sync
   ```

2. Check logs:
   ```bash
   journalctl --user -u clipboard-sync -n 50
   ```

3. Verify SSH connection:
   ```bash
   ssh your_mac_user@your_mac_host "pbpaste"
   ```

### Images not syncing

1. Verify `imagecopy` exists on Mac:
   ```bash
   ssh your_mac_user@your_mac_host "ls ~/scripts/imagecopy"
   ```

2. Test imagecopy manually on Mac:
   ```bash
   ssh your_mac_user@your_mac_host "~/scripts/imagecopy /tmp/test.png"
   ```

### SSH connection issues

1. Ensure SSH key is set up:
   ```bash
   ssh-copy-id your_mac_user@your_mac_host
   ```

2. Test connection:
   ```bash
   ssh -o BatchMode=yes your_mac_user@your_mac_host "echo OK"
   ```

### Wayland issues

1. Verify Wayland clipboard tools work:
   ```bash
   echo "test" | wl-copy
   wl-paste
   ```

2. Check `WAYLAND_DISPLAY` environment variable:
   ```bash
   echo $WAYLAND_DISPLAY
   ```

## License

MIT
