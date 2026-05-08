# Clipboard Sync - Future Improvements

## Priority 1: Event-Driven Clipboard Monitoring (Eliminate Polling)

### Option A: Implement watch mode in wl-clipboard-rs
- **Status**: Not currently supported
- **Effort**: Medium
- **Impact**: Eliminates CPU usage when idle, instant response to changes
- **Reference**: [wl-clipboard-rs repo](https://github.com/YaLTeR/wl-clipboard-rs)

### Option B: Implement wlr-data-control event loop directly
- **Status**: Protocol available, requires implementation
- **Effort**: High
- **Impact**: Native event-driven clipboard control
- **Protocol**: [wlr-data-control-unstable-v1](https://github.com/bugaevc/wl-clipboard/blob/master/src/protocol/wlr-data-control-unstable-v1.xml)
- **Compositor Support**: Sway, Hyprland, KWin, Mutter (varies)
- **Note**: Would need to use wayland-client crate directly

---

## Priority 2: Relay Hardening

### Improve relay authentication and authorization
- **Status**: Partially implemented
- **Current**: Shared token + room isolation
- **Next**: Per-room credentials, rotating tokens, per-device ACLs

### Add message persistence / late join sync history
- **Status**: Partially implemented
- **Current**: Server remembers only the latest text and latest image per room
- **Next**: Bounded history, conflict resolution, optional durable storage

### Add production packaging
- **Status**: Not currently supported
- **Impact**: Easier install and release automation for relay server/client binaries

---

## Priority 3: macOS and Linux Event-Driven Monitoring

### macOS change notifications
- **Current**: Polling via `pbpaste` and `imagecopy`
- **Improvement**: Use NSPasteboard change count monitoring

### Linux change notifications
- **Current**: Polling via `wl-paste`
- **Improvement**: Use compositor event subscriptions where supported

---

## Completed

- [x] SSH Multiplexing (ControlMaster) — 20-100x latency improvement
- [x] Relay-based multi-device architecture with Go server/client, room isolation, and deduplication metadata

---

## Research Sources

### SSH Optimization
- SSH config docs: https://man7.org/linux/man-pages/man5/ssh_config.5.html
- Multiplexing benchmarks: 200-600ms → 10-50ms per connection

### Wayland Protocols
- wlr-data-control: https://github.com/bugaevc/wl-clipboard/blob/master/src/protocol/wlr-data-control-unstable-v1.xml
- ext-data-control-v1: https://wayland.app/protocols/ext-data-control-v1
- wl-clipboard watch mode: https://github.com/bugaevc/wl-clipboard

### WebSocket / Clipboard Sync Projects
- CrossPaste: https://github.com/CrossPaste/crosspaste-desktop
- ClipCascade: https://github.com/Sathvik-Rao/ClipCascade
- ClipHop: https://github.com/theopedapolu/ClipHop
