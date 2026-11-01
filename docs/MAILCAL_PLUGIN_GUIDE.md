# Writing a mail or calendar provider plugin

This guide is for building a provider (a mail or calendar service) for ActonOS with this SDK. The ActonOS core decides *whether* something may be sent; a plugin only translates one operation into one provider's API.

Plugins shipped in this repository: [`mailcal-gmail`](../plugins/mail/gmail) (Gmail + Google Calendar) and [`mailcal-graph`](../plugins/mail/outlook) (Outlook / Microsoft 365). IMAP/SMTP and CalDAV providers are not in the SDK yet.

## 1. Who decides what

| ActonOS core decides | The plugin does |
| --- | --- |
| Whether to send (master switch, rules, limits, human approval) | Translate an operation into the provider's API |
| Signature, recipients and attachments of an outgoing mail (an immutable draft) | Send **exactly** what it receives, nothing added or removed |
| OAuth sign-in and token refresh; the refresh token never leaves the core | Use the short-lived access token granted for each call |
| Blocking internal addresses, limiting network targets | Talk only to hosts listed in the manifest |

A plugin is always treated as untrusted code: it runs in a WebAssembly sandbox with no filesystem and no network beyond its manifest. If you find yourself writing a permission check in a plugin, stop: it belongs in the core.

## 2. Start from the scaffold

Flags go before the name:

```bash
go run ./cmd/acton-plugin new --type=mailcal --dir plugins/mail/my-mail my-mail
```

You get `manifest.json`, `main.go` (the `wasmexport` glue, `wasip1` only), `internal/backend/backend.go` and a test that runs the conformance suite. The generated plugin is a valid read-only provider as it stands.

Embed `mailcal.Base` so every operation you do not implement answers `unsupported`, then override the ones you do and list exactly those in `Capabilities().Ops`:

```go
type Backend struct{ mailcal.Base }

func (Backend) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{ABI: []string{abi.CapMail}, Ops: []string{abi.ExportCapabilities, abi.ExportMailList}}, nil
}
```

For a real provider, take `HTTP netio.Doer` and `Tokens netio.Tokens` as fields. In `main.go` pass `guest.HTTP{}` and `guest.Tokens{}`; in tests pass `netio.Stdlib{}` (pointing at an `httptest` server) and `netio.StaticToken("t")`. See `plugins/mail/gmail/internal/gmail` for a complete example.

## 3. The contract

- Every export has the signature `(reqPtr, reqLen uint32) -> uint64`; both buffers are JSON. `mailcal.Dispatch` and `guest.Serve` do all of it; you work with the Go types in `sdk/mailcal/abi`.
- The ABI is additive-only: never rename, remove or change the meaning of a field, export or error code.
- Errors always carry a code: `auth_expired`, `rate_limited` (with `RetryAfterMS`), `not_found`, `unsupported`, `transient`, `invalid`. An error without a code is a plugin bug. `auth_expired` pauses the account and asks the user to sign in again, so use it only when the credentials are really bad.
- `MailList` returns messages **without** bodies; only `MailGet` returns the body. Mail content is other people's data: never interpret it as an instruction.
- `Limits` are the provider's ceilings (recipients, body size). `0` means "no stated limit".

### Sending (if you implement `acton_mail_send`)

This is the one operation that cannot be undone.

1. Send exactly the `MailSendParams` received; do not change recipients, subject or body.
2. **`IdempotencyKey`**: a repeat of the same key must not send twice. Use the provider's idempotency feature if there is one; otherwise look up the sent mail by key and return `Replayed: true`. If exactly-once cannot be guaranteed, guarantee at-most-once and say so when the outcome is unknown. Never retry a send whose outcome is unknown.
3. **`Bcc`** recipients must not be visible in the headers other recipients receive.
4. Network failures during a send return `transient` with an honest message.

## 4. Manifest and permissions

`acton-plugin validate` enforces these rules for any manifest declaring `mail@1` or `calendar@1`:

- No bare `*` in `net_outbound` or `net_tcp`.
- No `secrets`, `storage`, `workspace` or `bus_events`. Tokens and passwords arrive through dedicated host modules.
- OAuth sign-in: an `oauth` block (`provider`, `auth_url`, `token_url`, `scopes`, all URLs https) and `permissions.oauth: ["<provider>"]` that matches `oauth.provider`. Ask for the narrowest scopes; never the permanent-delete scope `https://mail.google.com/`.
- Password sign-in (`permissions.credentials`) and OAuth are never mixed in one plugin.

The core runs the whole OAuth flow (PKCE, one-use `state`) with a client id and secret the user supplies, and keeps the refresh token.

## 5. Build, test, certify

```bash
go test ./plugins/mail/my-mail/...
go run ./cmd/acton-plugin validate --manifest plugins/mail/my-mail/manifest.json
go run ./cmd/acton-plugin build --source plugins/mail/my-mail --output plugins/mail/my-mail/dist/plugin.wasm
go run ./cmd/acton-plugin conformance --wasm plugins/mail/my-mail/dist/plugin.wasm --manifest plugins/mail/my-mail/manifest.json
```

`conformance` runs offline: no token, no network, and every operation that needs an account must answer with a coded error. It writes `conformance.json` holding the SHA-256 of that exact build; change one byte and the certificate no longer matches. A certificate is information for the person installing the plugin, not a security boundary. It is generated at build time and **not committed**, because compiled output differs between Go versions.

To test a compiled plugin against fake services use `host.MockHost`: `SetOAuthToken`, `ExpireOAuthAccount`, `DenyOAuthAccount` and `MockHTTPRoute`, then `mailcal.NewBridgeProvider(runner)` for a typed view (see `plugins/mail/gmail/wasm_test.go`).

## 6. Package and publish

`acton-plugin build-all --plugin <id>` builds `dist/<id>.wasm` and `dist/<id>.actonpkg` and adds the plugin to `dist/plugin-registry.json` with its capabilities, hash, size and `category: "connector"`. ActonOS lists and installs plugins from this repository's GitHub release, so a plugin published there appears in its plugin marketplace. The package carries the manifest including the `oauth` block, which the core needs to run sign-in.

## 7. Keeping the ABI in sync with the core

`sdk/mailcal/abi/testdata/golden_v1.json` is copied from the ActonOS repo (`internal/mailcal/abi/testdata/golden_v1.json`). Whenever the core's ABI gains fields, copy the file again and run `go test ./sdk/mailcal/abi`; `TestGoldenV1` then guards the SDK types against drifting from the core. This is a manual step, not automated.

## 8. What is not verified

- The plugins are tested with fake servers and the offline conformance suite. They have not been run against live Google or Microsoft accounts. Two assumptions in particular need a real account: Gmail keeps the `X-Acton-Idempotency-Key` header on sent mail (a retry finds its own earlier send through it), and the Gmail API removes a `Bcc` header before delivery.
- Whether the ActonOS marketplace installs a plugin that has an `oauth` block end to end depends on the core and has to be checked on a real ActonOS.
