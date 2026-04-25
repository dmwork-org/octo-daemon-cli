<div align="center">

# octo-daemon-cli

Octo Agent Runtime 守护进程 — 自动检测本机 AI Agent 并上报状态到 Octo 服务端

[简体中文](./README.md) | [English](./README_EN.md)

</div>

---

## 简介

`octo-daemon` 是一个轻量级守护进程，安装在你的电脑或服务器上，自动检测本机已安装的 AI Agent CLI（Claude Code、OpenClaw、Hermes、Codex），并将设备和 Agent 信息上报到 Octo 服务端，在 Web 页面实时展示。

## 安装

```bash
go install github.com/dmwork-org/octo-daemon-cli@latest
```

要求 Go 1.20+，支持 macOS、Linux、Windows。

## 使用

### 获取 API Key

在 Octo 中向 BotFather 发送 `/daemon` 命令，会返回包含 API Key 和服务器地址的完整启动命令。

### 启动

```bash
octo-daemon start --api-key "uk_your_api_key" --api-url "http://your-server:8090"
```

### 查看状态

```bash
octo-daemon status
```

### 停止

```bash
octo-daemon stop
```

## 支持的 Agent

| Agent | 检测方式 | 状态判定 | 附加信息 |
|-------|---------|---------|---------|
| Claude Code | `claude --version` | 安装即在线 | — |
| Codex | `codex --version` | 安装即在线 | — |
| OpenClaw | `openclaw --version` | Gateway 运行 = 在线 | Agent 列表、插件列表 |
| Hermes | `hermes --version` | 安装即在线 | — |

## 工作原理

1. 启动时并行探测 PATH 中的 Agent CLI，秒级完成注册
2. OpenClaw 深度检测：gateway 运行状态、agent 配置列表、已安装插件
3. 每 15 秒发送心跳保持在线状态
4. 每 60 秒重新检测，发现变化（版本更新、agent 增减、gateway 启停）自动更新
5. 关闭时（Ctrl+C）优雅注销所有 runtime
6. 服务端 45 秒未收到心跳自动标记离线

## 本地数据

数据存储在 `~/.octo-daemon/` 目录：

| 文件 | 用途 |
|------|------|
| `daemon.id` | 机器唯一标识（UUID，首次生成后永久保留） |
| `daemon.lock` | 文件锁，防止重复启动 |
| `daemon.pid` | 当前进程 PID |

## 从源码构建

```bash
git clone https://github.com/dmwork-org/octo-daemon-cli.git
cd octo-daemon-cli
make build
```

交叉编译：

```bash
GOOS=linux GOARCH=amd64 make build     # Linux
GOOS=windows GOARCH=amd64 make build   # Windows
GOOS=darwin GOARCH=arm64 make build    # macOS Apple Silicon
```

## 许可证

MIT
