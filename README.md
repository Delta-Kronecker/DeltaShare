# DeltaShare

SOCKS5 proxy chain — share your internet connection with other devices.

## How it works

```
Remote Client ──▶ DeltaShare (:7373) ──▶ V2RayN (:10808) ──▶ Internet
```

DeltaShare listens on all network interfaces and forwards SOCKS5 traffic to a local upstream proxy (e.g. V2RayN on 127.0.0.1:10808).

## Features

- SOCKS5 proxy chaining (client -> DeltaShare -> upstream proxy -> internet)
- Optional username/password authentication
- IPv4, IPv6, and domain name support
- Startup banner with all available network addresses
- Live connection monitoring with real-time bandwidth tracking
- Session summary on shutdown
- Works with Telegram, V2RayNG, browsers, curl, and any SOCKS5 client

## Build

```bash
# Linux
go build -o deltashare .

# Windows
GOOS=windows GOARCH=amd64 go build -o deltashare.exe .
```

## Usage

```bash
# Basic (no auth) - listens on all interfaces
deltashare -listen 0.0.0.0:7373 -upstream 127.0.0.1:10808

# With username/password auth
deltashare -listen 0.0.0.0:7373 -upstream 127.0.0.1:10808 -user myuser -pass mypass

# Set public IP manually for display
deltashare -listen 0.0.0.0:7373 -upstream 127.0.0.1:10808 -ip 203.0.113.1
```

## Startup Output

```
╔════════════════════════════════════════════════════╗
║              DeltaShare v0.2.0                     ║
╠════════════════════════════════════════════════════╣
║  Listening on  : 0.0.0.0:7373                     ║
║  SOCKS5 Proxy  : 192.168.1.100:7373               ║
║  Auth          : disabled                          ║
║  Upstream      : 127.0.0.1:10808                  ║
╠════════════════════════════════════════════════════╣
║  Available addresses:                              ║
║    * 192.168.1.100:7373 (recommended)             ║
║      169.254.58.54:7373                           ║
╠════════════════════════════════════════════════════╣
║  Example (Telegram):                               ║
║    SOCKS5: 192.168.1.100:7373                     ║
║  Example (V2RayNG):                                ║
║    Address: 192.168.1.100                          ║
║    Port   : 7373                                   ║
║  Example (curl):                                   ║
║    curl --socks5 192.168.1.100:7373 https://...   ║
╚════════════════════════════════════════════════════╝
```

## Live Monitoring

Every 15 seconds, active connections are displayed:

```
── Connections: 2 active / 5 total ──
   ID     Client   Destination              Upload     Download   Time
   #1     10.0.0.  google.com:443           12.3KB     45.6KB     2m30s
   #3     10.0.0.  github.com:443           1.2KB      89.1KB     45s
── Total: ↑234.5KB  ↓1.2MB ──
```

## Client Setup

Connect any SOCKS5-capable app to your machine's IP on port 7373.

**Telegram:** Settings -> Data and Storage -> Proxy Settings -> Add SOCKS5 Proxy
**V2RayNG:** Add Server -> Type: SOCKS -> Address: your-ip, Port: 7373
**Browser:** Set SOCKS5 proxy to your IP, port 7373

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-listen` | `:7373` | Address to listen on (use `0.0.0.0:7373` for all interfaces) |
| `-ip` | *(auto-detect)* | Public IP for display |
| `-upstream` | `127.0.0.1:10808` | Upstream SOCKS5 proxy (V2RayN) |
| `-user` | *(empty)* | Username for client auth (empty = no auth) |
| `-pass` | *(empty)* | Password for client auth |

## Requirements

- V2RayN (or any SOCKS5 proxy) running on `127.0.0.1:10808`
- Port 7373 open on firewall
- Clients on the same network (LAN) or with port forwarding configured

## Downloads

Pre-built binaries are available on the [Releases](https://github.com/Delta-Kronecker/DeltaShare/releases) page.
