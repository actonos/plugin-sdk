# ActonOS Plugin CLI Reference (`acton-plugin`)

The `acton-plugin` command-line utility facilitates the entire lifecycle of developing WebAssembly plugins for ActonOS.

---

## Commands

### `new` / `init`
Scaffolds a new plugin directory from templates.
```bash
acton-plugin new [--type=tool|channel|connector|mailcal] [--dir=target_directory] <name>
```
Flags must come before the name (the CLI uses Go's `flag` package). `--type=mailcal` scaffolds a mail provider that already passes `conformance`; create it inside this repository, for example `--dir=plugins/mail/my-mail`.

### `build`
Compiles the Go plugin into WebAssembly using the WASI reactor shared-library target.
```bash
acton-plugin build [--source=.] [--output=dist/plugin.wasm] [--tinygo]
```

### `validate`
Validates `manifest.json` against semantic schemas, checks capability consistency, and warns about over-permissive domain egress.
```bash
acton-plugin validate [--manifest=manifest.json]
```

### `conformance`
Runs the mail/calendar contract suite offline (no token, no network) against a compiled provider and writes `conformance.json` (with the SHA-256 of that exact `.wasm`) next to it. Only for plugins that declare `mail@1` or `calendar@1`.
```bash
acton-plugin conformance [--wasm=dist/plugin.wasm] [--manifest=manifest.json]
```

### `test`
Executes local unit/integration tests on the compiled WASM binary inside an embedded **Wazero Mock Host**, complete with simulated Vault, KV Storage, Event Bus, and Network Firewall.
```bash
acton-plugin test [--wasm=dist/plugin.wasm] [--manifest=manifest.json] [--tool=tool_name] [--input='{"param":"value"}']
```

### `sign`
Generates Ed25519 keypairs and creates cryptographic signatures for plugin distribution.
```bash
# Generate keypair
acton-plugin sign --gen-key --key=private.key

# Sign manifest + wasm
acton-plugin sign --key=private.key [--manifest=manifest.json] [--wasm=dist/plugin.wasm] [--output=dist/signature.sig]
```

### `pack`
Packages manifest, compiled bytecode, cryptographic signature, and documentation into a single `.actonpkg` bundle.
```bash
acton-plugin pack [--manifest=manifest.json] [--wasm=dist/plugin.wasm] [--sig=dist/signature.sig] [--output=bundle.actonpkg]
```
