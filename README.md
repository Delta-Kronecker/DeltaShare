# DeltaShare

SOCKS5 proxy chain — share your internet connection with other devices.

## How it works

```
Remote Client ──▶ DeltaShare (:7373) ──▶ V2RayN (:10808) ──▶ Internet
```

DeltaShare listens on a public IP/port and forwards all traffic to a local SOCKS5 proxy (e.g. V2RayN running on 127.0.0.1:10808).

## Features

- SOCKS5 proxy chaining (client -> DeltaShare -> upstream proxy -> internet)
- Optional username/password authentication
- IPv4, IPv6, and domain name support
- Startup banner with connection info
- Live connection monitoring (upload/download/destination per connection)
- Graceful shutdown with session summary
- Auto-detect public IP (or set manually with `-ip`)

## Build

```bash
# Linux
go build -o deltashare .

# Windows
GOOS=windows GOARCH=amd64 go build -o deltashare.exe .
```

## Usage

```bash
# Basic (no auth)
deltashare -listen 0.0.0.0:7373 -upstream 127.0.0.1:10808

# With username/password auth
deltashare -listen 0.0.0.0:7373 -upstream 127.0.0.1:10808 -user myuser -pass mypass

# Set public IP manually
deltashare -listen 0.0.0.0:7373 -upstream 127.0.0.1:10808 -ip 203.0.113.1
```

## Startup Output

```
╔══════════════════════════════════════════════╗
║            DeltaShare v0.1.0                ║
╠══════════════════════════════════════════════╣
║  SOCKS5 Address : 192.168.1.100:7373        ║
║  Auth           : enabled                    ║
║  Upstream       : 127.0.0.1:10808           ║
╠══════════════════════════════════════════════╣
║  Connect with:                               ║
║  curl --socks5 192.168.1.100:7373 ...       ║
╚══════════════════════════════════════════════╝
```

## Live Monitoring

Every 15 seconds, active connections are displayed:

```
── Connections: 2 active / 5 total ──
   ID     Destination                    Upload     Download   Time
   #1     google.com:443                 12.3KB     45.6KB     2m30s
   #3     github.com:443                 1.2KB      89.1KB     45s
── Total: ↑234.5KB  ↓1.2MB ──
```

On shutdown, a session summary is displayed:

```
╔══════════════════════════════════════════════╗
║              Session Summary                 ║
╠══════════════════════════════════════════════╣
║  Total Connections : 5                       ║
║  Total Upload      : 234.5KB                ║
║  Total Download    : 1.2MB                  ║
╚══════════════════════════════════════════════╝
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-listen` | `:7373` | Address to listen on (public-facing) |
| `-ip` | *(auto-detect)* | Public IP for display |
| `-upstream` | `127.0.0.1:10808` | Upstream SOCKS5 proxy (V2RayN) |
| `-user` | *(empty)* | Username for client auth (empty = no auth) |
| `-pass` | *(empty)* | Password for client auth |

## Client Setup

Other devices connect to your public IP on port 7373 as a SOCKS5 proxy.

Example with curl:
```bash
curl --socks5 192.168.1.100:7373 https://example.com
```

Example with username/password:
```bash
curl --socks5 myuser:mypass@192.168.1.100:7373 https://example.com
```

Browser: Set SOCKS5 proxy to your public IP, port 7373.

## Requirements

- V2RayN (or any SOCKS5 proxy) running on `127.0.0.1:10808`
- Port 7373 open on your firewall
- Public IP (or port forwarding configured)

## Downloads

Pre-built binaries are available on the [Releases](https://github.com/Delta-Kronecker/DeltaShare/releases) page.
