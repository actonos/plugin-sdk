# ActonOS Plugin SDK (`acton-plugin-sdk`)

[![Go Version](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev)
[![WASM Runtime](https://img.shields.io/badge/WASM-Wazero%20Pure%20Go-blueviolet)](https://wazero.io)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

The official **Software Development Kit (SDK)** and **Developer CLI Toolchain** for building WebAssembly (WASM) plugins for [ActonOS](https://github.com/actonos/actonos).

ActonOS uses a sandboxed **WasmLoader** architecture to safely run third-party extensions:
- 🛠️ **Agent Tools**: Custom functions and cognitive tools callable by ReAct agent swarms with automatic JSON schema generation.
- 💬 **Chat Channels**: Bi-directional chat adapters for Telegram, Discord, Slack, WhatsApp, and Webhooks.
- 🔌 **SaaS Connectors**: Secure integrations with GitHub, Notion, Linear, Airtable with Hardware Vault token brokering.

---

## Key Features

- ⚡ **100% Pure Go & Zero CGO**: Runs cleanly under standard `wasip1` / `wasm` target with native Go toolchain or TinyGo.
- 🛡️ **Fail-Closed Sandboxing**: Capability-based security model enforcing Domain Egress Firewall, Hardware Vault isolation, and SQLite KV storage boundaries.
- 🧪 **Built-in Mock Host**: Test WASM plugins locally using an embedded Wazero sandbox harness without needing a live ActonOS daemon.
- 🔐 **Cryptographic Signing**: Ed25519 public-key signature verification ensuring plugin code integrity and tamper-proofing.
- 🚀 **One-Stop Developer CLI (`acton-plugin`)**: Complete workflow for scaffolding (`new`), building (`build`), testing (`test`), validating (`validate`), signing (`sign`), and packaging (`pack`).

---

## Quickstart in 30 Seconds

### 1. Build the Developer CLI
```bash
go build -o acton-plugin ./cmd/acton-plugin/
```

### 2. Create a New Plugin
```bash
./acton-plugin new my-weather-tool --type=tool
cd my-weather-tool
```

### 3. Build, Test, and Package
```bash
# Validate manifest permissions
../acton-plugin validate

# Compile to WebAssembly
../acton-plugin build

# Test inside local Wazero mock sandbox
../acton-plugin test --input='{"city":"Tokyo"}'

# Sign and create distribution bundle
../acton-plugin sign --gen-key
../acton-plugin sign
../acton-plugin pack
```

---

## Architecture & Repository Structure

```
ActonOS-Plugin-SDK/
├── sdk/                     # Core Guest Plugin SDK (WASM ABI, Host API clients, reflection schema)
│   ├── abi/                 # Low-level linear memory management and syscall imports
│   ├── context.go           # HTTPClient, VaultClient, KVStorage, EventBus, Logger
│   ├── schema.go            # Struct-to-JSONSchema reflection engine
│   ├── tool.go              # Typed Tool abstraction
│   ├── channel.go           # Channel adapter interface
│   ├── connector.go         # SaaS connector interface & action routing
│   └── plugin.go            # WASM lifecycle & export dispatchers
├── host/                    # Wazero-based Mock Host Test Harness
│   ├── host.go              # Host syscall implementations (HTTP, Vault, Storage, Bus, Log)
│   └── runner.go            # WASM module loader and executor
├── cmd/acton-plugin/        # Developer CLI Toolchain
├── plugins/                 # Built-in Official Plugins
│   ├── channels/            # Chat channels (Discord, Slack, Telegram, WhatsApp, Zalo)
│   └── saas/                # SaaS connectors (Figma, GitHub, Google, Jira, Linear, Notion, Slack)
├── spec/                    # Formal ABI & Manifest specifications
└── docs/                    # Complete developer documentation
```

---

## Documentation

- [Getting Started Guide](docs/GETTING_STARTED.md)
- [Developer Guide (Tools, Channels, Connectors)](docs/DEVELOPER_GUIDE.md)
- [CLI Reference Guide](docs/CLI_GUIDE.md)
- [Security Best Practices](docs/SECURITY_BEST_PRACTICES.md)
- [WASM ABI Specification](spec/WASM_ABI.md)

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
