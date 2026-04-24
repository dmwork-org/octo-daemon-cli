# octo-daemon-cli

Octo Agent Runtime Daemon — detects local AI agent runtimes (Claude Code, OpenClaw, Hermes, Codex) and reports their status to the Octo server.

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

## Supported Runtimes

| Provider | Binary | Detection |
|----------|--------|-----------|
| Claude Code | `claude` | `claude --version` |
| Codex | `codex` | `codex --version` |
| OpenClaw | `openclaw` | `openclaw --version` + gateway status + agent list + plugins |
| Hermes | `hermes` | `hermes --version` |

## How It Works

1. Probes `$PATH` for supported agent CLI binaries and runs `--version` to get version info
2. For OpenClaw: detects gateway running status, enumerates configured agents, and lists installed plugins
3. Registers detected runtimes with the Octo server via REST API
4. Sends heartbeats every 15 seconds to maintain online status
5. Re-detects every 60 seconds and re-registers on any change (new agents, version updates, gateway status change)
6. On shutdown (SIGTERM / SIGINT / Ctrl+C), deregisters all runtimes gracefully
7. Server-side sweeper marks runtimes offline if no heartbeat received for 45 seconds

## Data Storage

Runtime data is stored locally in `~/.octo-daemon/`:

| File | Purpose |
|------|---------|
| `daemon.id` | Persistent machine UUID (generated once, survives hostname changes) |
| `daemon.lock` | File lock for single-instance protection |

## Cross-Platform

- **macOS / Linux**: Full support
- **Windows**: Full support (`go install` produces `octo-daemon.exe`)

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
```

## License

MIT
