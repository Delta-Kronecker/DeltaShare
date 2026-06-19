# DeltaShare

SOCKS5 proxy chain — share your internet connection with other devices.

```
Remote Client ──▶ DeltaShare (:7373) ──▶ V2RayN (:10808) ──▶ Internet
```

## Features

- SOCKS5 proxy chaining
- Optional username/password authentication
- Professional TUI dashboard (tview/tcell - works on Windows, Linux, Mac)
- Real-time bandwidth tracking
- Last 10 connections display
- Auto-detect best LAN IP
- Telegram & V2RayNG connection links

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
