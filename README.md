# DeltaShare

SOCKS5 proxy chain — share your internet connection with other devices.

## How it works

```
Remote Client ──▶ DeltaShare (:7373) ──▶ V2RayN (:10808) ──▶ Internet
```

DeltaShare listens on a public IP/port and forwards all traffic to a local SOCKS5 proxy (e.g. V2RayN running on 127.0.0.1:10808).

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
deltashare.exe -listen 0.0.0.0:7373 -upstream 127.0.0.1:10808

# With username/password auth on client side
deltashare.exe -listen 0.0.0.0:7373 -upstream 127.0.0.1:10808 -user myuser -pass mypass
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-listen` | `:7373` | Address to listen on (public-facing) |
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
