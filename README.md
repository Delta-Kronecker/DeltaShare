# DeltaShare

SOCKS5 proxy chain — share your internet connection with other devices.

## How it works

```
Remote Client ──▶ DeltaShare (:7373) ──▶ V2RayN (:10808) ──▶ Internet
```

## Features

- SOCKS5 proxy chaining
- Optional username/password authentication
- Colorful dashboard UI with progress bars
- Real-time bandwidth tracking
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
  ╭──────────────────────────────────────────────────────────╮
  │    ■  DeltaShare  v0.4.0                                 │
  │                                                          │
  │  SOCKS5    10.14.2.15:7373                               │
  │  Upstream  127.0.0.1:10808                               │
  │  Auth      off                                           │
  │  Uptime    2h 15m 30s                                    │
  │                                                          │
  ├──────────────────────────────────────────────────────────┤
  │                                                          │
  │  ↑ Upload   ████████████████░░░░░░░░░░░░░░░░              │
  │  ↓ Download ████████████████████████████░░░░              │
  │                                                          │
  ├──────────────────────────────────────────────────────────┤
  │  Active 3        Total 47                                │
  │  ↑ 15.30 MB      ↓ 128.70 MB                            │
  │                                                          │
  ╰──────────────────────────────────────────────────────────╯
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
