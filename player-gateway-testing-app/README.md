# Amazon GameLift Servers player gateway testing app

A UDP relay testing tool for validating Amazon GameLift Servers player gateway integration scenarios.

## Table of contents

- [Overview](#overview)
- [Features](#features)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Configuration](#configuration)
  - [Required flags](#required-flags)
  - [Optional flags](#optional-flags)
  - [Examples](#examples)
- [Token format](#token-format)
- [Runtime configuration](#runtime-configuration)
  - [set-degradation](#set-degradation)
  - [replace-token](#replace-token)
  - [get-player-connection-details](#get-player-connection-details)
- [How it works](#how-it-works)
  - [Traffic flow](#traffic-flow)
- [Testing](#testing)
- [Troubleshooting](#troubleshooting)
- [License](#license)
- [Support](#support)

## Overview

This application simulates the player gateway UDP relay infrastructure to test network connectivity and packet handling between game clients and servers. It helps validate your game's integration with Amazon GameLift Servers player gateway before deployment.

## Features

- Multiple UDP endpoint simulation (1-10 configurable endpoints)
- Token-based packet validation
- Dynamic client connection management
- Runtime configuration via special packets
- Packet degradation simulation for network testing
- Invalid packet reporting

## Prerequisites

- Go 1.22 or later (https://go.dev/dl/)
- An active game session with an IP and port
- Network access between test clients and game server

## Installation

### Build from source

```bash
go build
```

This creates the `player-gateway-testing-app` executable.

## How it works

The testing app creates a relay infrastructure that mimics Amazon GameLift Servers player gateway:

- **Client-side relays**: N endpoints (configurable) that receive traffic from the game clients
- **Server-side relay**: Single endpoint that routes traffic to your game server. Each player will be allocated a port that will send and receive traffic from the game server

### Traffic flow

**Client → Server:**
1. The game client sends packets prepended with the token to a client-side endpoint on the testing app
2. The testing app validates the token and routes the packet to the game server from its server-side endpoint with a port specific to the player

**Server → Client:**
1. The game server sends packets to the server-side endpoint on the testing app for a specific player's port
2. The testing app routes the traffic to the appropriate game client via one of it's client-side endpoints

## Quick start

Run the testing app pointing to your game server:

```bash
./player-gateway-testing-app \
  --game-server-ip 192.168.1.100 \
  --game-server-port 8000
```

The app will start UDP endpoints on localhost (127.0.0.1) by default and will route incoming traffic to the game server.
On startup, the app will log the list of client-side endpoints. The game clients should use these endpoints to connect to the testing app.

## Configuration

### Required flags

| Flag | Short | Description |
|------|-------|-------------|
| `--game-server-ip` | `-s` | IP address or hostname of your game server |
| `--game-server-port` | `-p` | Port your game server listens on |

### Optional flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--testing-app-ip` | `-i` | `127.0.0.1` | IP address for the testing app to listen on |
| `--udp-endpoint-count` | `-u` | `3` | Number of UDP endpoints per player (1-10) |
| `--tool-port-range` | `-r` | (auto-generated) | Port range for allocation: `<start>-<end>` |
| `--player-count` | `-n` | `1` | Number of players (1 token is generated per player) |
| `--report-file-path` | `-f` | `./report.txt` | Path for invalid packet reports |
| `--headless` | `-H` | false | Run the application without the UI |

### Examples

Basic usage with custom endpoint count:
```bash
./player-gateway-testing-app \
  --game-server-ip 10.0.1.50 \
  --game-server-port 7777 \
  --udp-endpoint-count 5
```

With custom port range:
```bash
./player-gateway-testing-app \
  --game-server-ip game.example.com \
  --game-server-port 9000 \
  --tool-port-range 10000-10100
```

## Runtime configuration

The testing app's behavior can be modified while it's running using CLI subcommands.
The testing app UI will reflect any changes made by the subcommands during runtime.

### set-degradation

Simulate packet loss by configuring drop percentage (0-100).

```bash
./player-gateway-testing-app set-degradation \
  --degradation-percentage 50 \
  --port 8000
```

Flags:
- `--degradation-percentage`: Percentage of packets to drop (0-100) (required)
- `--port`: Port of the running testing app endpoint to modify (required)

Examples:
```bash
# Disable packet loss
./player-gateway-testing-app set-degradation --degradation-percentage 0 --port 8000

# Drop 25% of packets
./player-gateway-testing-app set-degradation --degradation-percentage 25 --port 8000

# Drop all packets
./player-gateway-testing-app set-degradation --degradation-percentage 100 --port 8000
```

### replace-token

Generate a new token and replace the existing token for a specific player. The old token is invalidated.

```bash
./player-gateway-testing-app replace-token \
  --player-number 1
```

Flags:
- `--player-number`: The player number (required)
- `--port`: Any port of the running testing app (default: 8000)

### get-player-connection-details

Get connection details for specified players. This simulates the GetPlayerConnectionDetails API and returns the token and endpoints for each player.

```bash
./player-gateway-testing-app get-player-connection-details \
  --player-numbers 1,2,3
```

Flags:
- `--player-numbers`: Comma-separated list of player numbers (required)
- `--port`: Any port of the running testing app (default: 8000)

Example output:
```json
[
  {
    "PlayerNumber": "1",
    "Endpoints": [
      {"IpAddress": "127.0.0.1", "Port": 8000},
      {"IpAddress": "127.0.0.1", "Port": 8001},
      {"IpAddress": "127.0.0.1", "Port": 8002}
    ],
    "PlayerGatewayToken": "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMg==",
    "Expiration": 1700000000
  }
]
```

## Testing

Run the test suite:

```bash
go test ./...
```

## Troubleshooting

### Port allocation errors

If you see port allocation failures, specify a custom port range:
```bash
--tool-port-range 20000-20100
```

### Token validation failures

Check that your client packets include the correct token prefix. Review `report.txt` for details on invalid packets.

### Connection issues

Verify:
- Game server is running and accessible
- Firewall rules allow UDP traffic
- IP address and port are correct

## License

This project is licensed under the Apache License 2.0. See the LICENSE file for details.

## Support

For issues and questions, please refer to the [Amazon GameLift Servers documentation](https://docs.aws.amazon.com/gameliftservers/) or contact AWS Support.
