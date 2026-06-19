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
- Clean terminal UI with real-time connection monitoring
- Only shows usable LAN IPs (hides link-local and WSL addresses)
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

## Interface

```
  ╔═══════════════════════════════════════════════════╗
  ║             DeltaShare v0.3.0                    ║
  ╠═══════════════════════════════════════════════════╣
  ║  Address  : 10.14.2.15:7373                      ║
  ║  Auth     : disabled                             ║
  ║  Upstream : 127.0.0.1:10808                      ║
  ╠═══════════════════════════════════════════════════╣
  ║  Telegram : Settings > Proxy > SOCKS5             ║
  ║             10.14.2.15:7373                      ║
  ║  V2RayNG  : Type SOCKS5                          ║
  ║             10.14.2.15:7373                      ║
  ║  curl     : --socks5 <addr>:<port> <url>         ║
  ╚═══════════════════════════════════════════════════╝

  ┌──────┬───────────────┬────────────────────────────┬──────────┬──────────┬──────────┐
  │  ID  │    Client     │       Destination          │  Upload  │ Download │   Time   │
  ├──────┼───────────────┼────────────────────────────┼──────────┼──────────┼──────────┤
  │ #1   │ 192.168.1.5   │ google.com:443             │   12.3KB │   45.6KB │   2m30s  │
  │ #2   │ 192.168.1.8   │ github.com:443             │    1.2KB │   89.1KB │     45s  │
  └──────┴───────────────┴────────────────────────────┴──────────┴──────────┴──────────┘
  Total: 5 connections  |  ↑ 234.5KB  |  ↓ 1.2MB
```

## Client Setup

**Telegram:** Settings -> Data and Storage -> Proxy Settings -> Add SOCKS5 Proxy

**V2RayNG:** Add Server -> Type: SOCKS -> Address: your-ip, Port: 7373

**Browser:** Set SOCKS5 proxy to your IP, port 7373

**curl:** `curl --socks5 10.14.2.15:7373 https://example.com`

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
