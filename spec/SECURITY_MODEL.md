# ActonOS Plugin Security & Sandbox Model

This document specifies the security boundaries, capability-based access control, cryptographic verification, and memory safety rules enforced for all ActonOS WASM Plugins.

---

## 1. Zero-Trust Sandboxing Architecture

ActonOS executes guest plugins inside an isolated **WebAssembly Virtual Sandbox** (managed by Wazero):

1. **Linear Memory Isolation**:
   - The plugin code cannot access memory outside of its own allocated WASM memory pages.
   - Host buffers and OS kernel calls are completely inaccessible directly.
2. **Cgroups & Epoch Timeout**:
   - Plugins are executed under a strict execution deadline (default: 10s per tool call).
   - CPU loops are safely halted by Wazero's epoch interruption mechanism without crashing the daemon.
3. **No Direct Socket Access**:
   - WASM guest code cannot open raw TCP/UDP sockets.
   - All network traffic MUST flow through the Host HTTP Proxy (`acton_net.http_request`), which enforces strict DNS/TLS validation and domain whitelisting.

---

## 2. Capability-Based Permission Model

A plugin MUST declare all required permissions explicitly in its `manifest.json`. Any undeclared resource request is rejected by default (*Fail-Closed*):

### 2.1. Network Egress Firewall (`net_outbound`)
- Only domains declared in `permissions.net_outbound` are permitted.
- Wildcards are supported (e.g. `*.telegram.org`, `api.github.com`).
- Localhost (`127.0.0.1`, `localhost`, `10.0.0.0/8`, `192.168.0.0/16`) access is strictly blocked to prevent Server-Side Request Forgery (SSRF) to the ActonOS internal API.

### 2.2. Hardware Vault Brokering (`secrets`)
- Plugins never receive master keys.
- If a plugin requires a token (e.g. `telegram_bot_token`), it requests the secret by key name through `acton_vault.get_secret`.
- If the secret is not listed in `permissions.secrets` or the user has not granted permission, the host returns an empty authorization error.

### 2.3. SQLite Key-Value Storage (`storage`)
- If enabled (`permissions.storage = true`), the plugin is allocated an isolated SQLite partition namespaced by its `plugin_id`.
- Plugins cannot read or overwrite keys belonging to other plugins or core ActonOS storage.

---

## 3. Cryptographic Package Signing & Integrity

Every `.actonpkg` bundle can be signed using **Ed25519** public-key cryptography:

1. **Hash Generation**: $H = \text{SHA-256}(\text{manifest.json} \mathbin{\Vert} \text{plugin.wasm})$.
2. **Signature**: $\sigma = \text{Ed25519\_Sign}(K_{\text{private}}, H)$.
3. **Verification**: During installation, ActonOS verifies $\sigma$ using the developer's public key or a trusted registry keystore. If the signature is invalid or the bytecode has been modified, installation is immediately aborted.
