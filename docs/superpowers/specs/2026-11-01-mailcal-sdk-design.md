# Mail and calendar provider plugins in the Plugin SDK

Date: 2026-11-01 · Status: **design approved, awaiting spec review** · Scope: first round (Google and Microsoft)

## 1. Goal and decisions

Let anyone, including the ActonOS team, build mail and calendar provider plugins with this SDK: a stable contract, a support library, build/test/certification tooling, and reference plugins for the most popular providers.

The ActonOS core already ships the host side (`mail@1` / `calendar@1` ABI, rules engine, approvals, OAuth flow) and four provider plugins inside its own repo (`internal/mailcal/plugins/`). Those cannot be imported from this repo, so this work ports them here.

**Decisions made with the project owner**

| Topic | Decision |
|:---|:---|
| Round scope | SDK support **plus Gmail/Google Calendar and Outlook/Microsoft 365 (Graph)**. IMAP/SMTP and CalDAV come later. |
| Source of truth | **This repo.** Plugins are built here as `.actonpkg`; ActonOS later drops its `internal` copies and installs packages from here (separate round). |
| Pattern | Sub-packages under `sdk/mailcal/` (not the root `sdk` package, not the Connector/Tool API). |
| `google-workspace` connector | Kept, with its mail and calendar actions marked deprecated. |

**Assumptions** (correct me if wrong)
- The core's ABI v1 is frozen and additive-only, so this SDK matches it byte for byte and does not redesign it.
- Users bring their own OAuth client; the core runs sign-in and holds the refresh token.
- The core, not the plugin, decides whether a message is sent. Plugins are untrusted code.

**Out of scope this round:** IMAP/SMTP, CalDAV, the `acton_tcp` and `acton_account` host modules, `icskit`, switching ActonOS to consume these packages, testing against live Google/Microsoft accounts.

## 2. Why not the existing Connector/Tool path

The core calls `acton_mail_*` / `acton_calendar_*` exports and routes writes through its rules and approval gate. A `Connector` is called through `acton_tool_execute` with a static token and bypasses that gate. So mailcal needs its own contract, and the exports must live in the plugin's `main` package (a `//go:wasmexport` requirement), which also rules out folding it into package `sdk`.

## 3. Package layout

| Path | Responsibility |
|:---|:---|
| `sdk/mailcal/abi` | Wire types, error codes, export names, address normalisation. Additive-only. Ships `testdata/golden_v1.json` copied from the core; a test fails if the types stop round-tripping it. |
| `sdk/mailcal` | `Backend` interface (typed view of the ABI), `Base` (answers `unsupported`), `Dispatch` (never panics, every failure is a coded error). |
| `sdk/mailcal/netio` | `Doer` and `Tokens` interfaces plus a stdlib implementation, so a provider is unit-testable natively. |
| `sdk/mailcal/guest` (`wasip1`) | `Alloc`, `Free`, `Serve`; `HTTP{}` over `acton_net`; `Tokens{}` over `acton_oauth`. |
| `sdk/mailcal/mimekit` | MIME build and parse (needed by Gmail). |
| `sdk/mailcal/conformance` | Contract test suite; `certificate` writes `conformance.json` holding the SHA-256 of the exact `.wasm`. |
| `plugins/mail/gmail` | id `mailcal-gmail`: Gmail and Google Calendar. |
| `plugins/mail/outlook` | id `mailcal-graph`: Outlook/Microsoft 365 mail and calendar. |

Plugin ids match the core's so the core needs no change to recognise them.

## 4. Tooling for plugin authors

- **Manifest.** `sdk.PluginManifest` and `spec/MANIFEST_SCHEMA.json` gain `capabilities: mail@1 | calendar@1`, an `oauth` block (`provider`, `auth_url`, `token_url`, `scopes`) and `permissions.oauth`. `permissions.credentials` and `permissions.net_tcp` are added to the schema now so it does not change again next round.
- **`acton-plugin validate`** enforces the core's rules: no bare `*` in `net_outbound` or `net_tcp`; providers do not request `secrets`, `storage`, `workspace` or `bus_events`; `oauth.provider` matches `permissions.oauth`; no plugin mixes OAuth and password sign-in.
- **`host.MockHost`** gains the `acton_oauth` module with `SetOAuthToken(account, token)` and the core's return codes (`-2` account not permitted, `-3` sign-in expired), plus a `CallMailcal(op, request)` helper.
- **`acton-plugin new --type mailcal`** scaffolds a plugin including `wasm/main.go`.
- **`acton-plugin conformance`** runs the suite offline against a `.wasm` and writes `conformance.json` next to it.

## 5. Plugins

Port the behaviour, not a simplification:

- **Gmail + Calendar.** Retry-safe send via the `X-Acton-Idempotency-Key` header and lookup, `Bcc` never exposed to other recipients, the narrowest scopes (`gmail.modify`, `calendar.events`, `calendar.freebusy`), mail delete moves to trash and never deletes permanently.
- **Graph.** Idempotency via an extended property for mail and `transactionId` for events; scopes `offline_access Mail.ReadWrite Mail.Send Calendars.ReadWrite`; mail delete moves to Deleted Items.

Both declare exactly the operations they implement; every other operation returns `unsupported`, which the conformance suite checks.

**Tests.** Unit tests use `httptest` fakes as in the core. Each plugin also gets a test that runs the compiled `.wasm` through `MockHost`, so the WASM ABI path is covered and not only the native one.

**Build and registry.** `build_all` produces `.wasm` and `.actonpkg` in `dist/` and lists them in `plugin-registry.json`. `conformance.json` is generated at build time and not committed, because compiled output differs between Go versions.

**Delivery through the marketplace.** ActonOS lists and installs plugins from this repo's GitHub release (`.../releases/latest/download/plugin-registry.json` and the `.actonpkg` files next to it). Today the four mailcal plugins live in the core repo and are not in that catalogue, so users cannot find, install, enable or disable them from the UI; they must run `make plugin-gmail` and copy files by hand. Moving them here fixes that. Requirements:
- Each registry entry declares `capabilities: ["mail@1", "calendar@1"]`, a correct `sha256` and `size_bytes`, and an explicit `category` (see risks).
- The `.actonpkg` carries the manifest **including the `oauth` block**, because the core needs it to run sign-in.
- The registry generator in `build_all` is updated for the new plugins.

## 6. The `google-workspace` connector

Unchanged in behaviour. `send_email` and `create_calendar_event` are marked deprecated in the manifest description and docs, pointing to `mailcal-gmail`. Drive and Docs actions keep working. Nothing is removed in this round.

## 7. Documentation and versioning

- New `docs/MAILCAL_PLUGIN_GUIDE.md`, derived from the core's guide with SDK import paths, including the step for re-syncing `golden_v1.json` from the core.
- Update `docs/BUILTIN_PLUGINS.md`, `docs/CLI_GUIDE.md`, `README.md` and `CHANGELOG.md`.
- Bump `VERSION` and `sdk/VERSION` to 1.5.0 (minor: additive API). The WASM ABI version stays the same.

## 8. Risks and honest limits

- **Unverified against live services.** Plugins are verified with fake servers and the offline conformance suite, as in the core. Two assumptions remain untested on real accounts: Gmail keeps the idempotency header on sent mail, and the Gmail API strips `Bcc` before delivery. The guide states this.
- **ABI drift.** The golden test only detects drift if `golden_v1.json` is re-synced from the core by hand. This is documented, not automated.
- **Size.** The `.wasm` files will be large because of the HTTP/JSON stack; the plugins are not tuned for size this round.
- **Marketplace behaviour is unverified.** The core's registry code categorises plugins only as `channel`, `connector` or `tool`, so a `mail@1` plugin would land in "tool" unless its entry sets `category`. Setting it in the registry avoids a core change, but whether the marketplace and installer accept a plugin with an `oauth` block end to end must be tried on a real ActonOS. If it needs a core change, that is reported and handled in the ActonOS follow-up round, not guessed here.
- **Duplication until ActonOS switches over.** For a time two copies exist (core `internal` and here). The follow-up round removes the core copy.

## 9. Success criteria

1. `go build ./...`, `go vet ./...` and `go test ./...` pass.
2. Both plugins pass the conformance suite on their compiled `.wasm` via `acton-plugin conformance`.
3. `acton-plugin validate` accepts both manifests and rejects each rule violation in section 4 (covered by tests).
4. `acton-plugin new --type mailcal` produces a plugin that builds and passes conformance without edits.
5. Both plugins appear in the generated `plugin-registry.json` with correct capabilities, hash, size and category, and their `.actonpkg` contains the `oauth` block (checked by a test on the package). Installing them from the ActonOS marketplace is verified manually and reported, as it depends on the core.
6. A hand-written provider using only `sdk/mailcal` and the guide reaches the same result without touching the SDK.
