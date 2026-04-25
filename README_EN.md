<div align="center">

# octo-daemon-cli

Octo Agent Runtime Daemon — automatically detects local AI agents and reports their status to the Octo server

[简体中文](./README.md) | [English](./README_EN.md)

</div>

---

## Introduction

`octo-daemon` is a lightweight daemon that runs on your computer or server, automatically detects installed AI Agent CLIs (Claude Code, OpenClaw, Hermes, Codex), and reports device and agent information to the Octo server for real-time visibility on the web dashboard.

## Install

```bash
go install github.com/dmwork-org/octo-daemon-cli@latest
```

Requires Go 1.20+. Supports macOS, Linux, and Windows.

## Usage

### Get your API key

In Octo, send `/daemon` to BotFather. It will return a ready-to-use start command with your API key and server URL.

### Start

```bash
octo-daemon start --api-key "uk_your_api_key" --api-url "http://your-server:8090"
```

### Status

```bash
octo-daemon status
```

### Stop

```bash
octo-daemon stop
```

## Supported Agents

| Agent | Detection | Status | Extra Info |
|-------|-----------|--------|-----------|
| Claude Code | `claude --version` | Installed = online | — |
| Codex | `codex --version` | Installed = online | — |
| OpenClaw | `openclaw --version` | Gateway running = online | Agent list, plugin list |
| Hermes | `hermes --version` | Installed = online | — |

## How It Works

1. On startup, probes PATH for agent CLIs in parallel — registers within seconds
2. For OpenClaw: detects gateway status, enumerates configured agents, lists installed plugins
3. Sends heartbeats every 15 seconds to maintain online status
4. Re-detects every 60 seconds and re-registers on any change (version updates, agent changes, gateway status)
5. On shutdown (Ctrl+C), gracefully deregisters all runtimes
6. Server-side sweeper marks runtimes offline if no heartbeat received for 45 seconds

## Data Storage

Runtime data is stored locally in `~/.octo-daemon/`:

| File | Purpose |
|------|---------|
| `daemon.id` | Persistent machine UUID (generated once, survives hostname changes) |
| `daemon.lock` | File lock for single-instance protection |
| `daemon.pid` | Current process PID |

## Build from Source

```bash
git clone https://github.com/dmwork-org/octo-daemon-cli.git
cd octo-daemon-cli
make build
```

Cross-compile:

```bash
GOOS=linux GOARCH=amd64 make build     # Linux
GOOS=windows GOARCH=amd64 make build   # Windows
GOOS=darwin GOARCH=arm64 make build    # macOS Apple Silicon
```

## License

MIT
