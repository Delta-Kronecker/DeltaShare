# DeltaShare

SOCKS5 proxy chain — share your internet connection with other devices.

## How it works

```
Remote Client ──▶ DeltaShare (:7373) ──▶ V2RayN (:10808) ──▶ Internet
```

## Features

- SOCKS5 proxy chaining
- Optional username/password authentication
- Colorful dashboard with real-time stats
- Last 10 connections display
- Auto-detect best LAN IP
- Works with Telegram, V2RayNG, browsers, curl

## Build

```bash
# Linux
go build -o deltashare .

# Windows
GOOS=windows GOARCH=amd64 go build -o deltashare.exe .
```

## Usage

```bash
deltashare -listen 0.0.0.0:7373 -upstream 127.0.0.1:10808
```

## Dashboard

```
  ╭───────────────────────────────────────────────────────────╮
  │  ■  DeltaShare  v0.5.0                                    │
  │                                                           │
  │  Address   10.14.2.15:7373                                │
  │  Upstream  127.0.0.1:10808                                │
  │  Auth      off                                            │
  │  Uptime    2m 0s                                          │
  │                                                           │
  ├───────────────────────────────────────────────────────────┤
  │                                                           │
  │  ↑ Upload   47.61 KB    ↓ Download 2.31 KB               │
  │  Active    3            Total     99                      │
  │                                                           │
  ├───────────────────────────────────────────────────────────┤
  │  Connect via:                                             │
  │  Telegram:  https://t.me/socks?server=10.14.2.15&port=7373│
  │  V2RayNG:   socks://10.14.2.15:7373                      │
  │                                                           │
  ├───────────────────────────────────────────────────────────┤
  │  Recent connections:                                      │
  │                                                           │
  │  ID     Destination                Upload     Download   │
  │  #99    149.154.167.41:443         530 B      178 B     │
  │  #98    149.154.167.92:443         10.1 KB    64.7 KB   │
  ╰───────────────────────────────────────────────────────────╯
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-listen` | `:7373` | Listen address |
| `-ip` | *(auto)* | Public IP for display |
| `-upstream` | `127.0.0.1:10808` | Upstream SOCKS5 proxy |
| `-user` | *(empty)* | Username for auth |
| `-pass` | *(empty)* | Password for auth |

## Requirements

- V2RayN or any SOCKS5 proxy on `127.0.0.1:10808`
- Port 7373 open on firewall

## Downloads

[Releases](https://github.com/Delta-Kronecker/DeltaShare/releases)
