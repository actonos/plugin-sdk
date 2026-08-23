# Security Best Practices for ActonOS Plugins

This guide outlines security recommendations for authoring safe, sandboxed, and privacy-respecting ActonOS plugins.

---

## 1. Apply the Principle of Least Privilege

1. **Specific Domain Egress**:
   - Never use `"net_outbound": ["*"]` in production.
   - Always declare specific API hostnames (e.g. `["api.github.com", "uploads.github.com"]` or `["*.telegram.org"]`).
2. **Explicit Secret Scoping**:
   - Only request secrets that your plugin actively needs.
   - Never log secret tokens to `ctx.Log()` or store plaintext tokens in KV storage.
3. **KV Storage Isolation**:
   - ActonOS automatically namespaces SQLite KV storage by plugin ID, ensuring plugins cannot tamper with each other's storage partitions.

---

## 2. Cryptographic Integrity

1. Always sign production builds with `acton-plugin sign`.
2. Keep your private key (`.key`) outside public repositories and CI artifacts.
3. Users and enterprise administrators can enforce signature verification in ActonOS to prevent unauthorized plugin executions.
