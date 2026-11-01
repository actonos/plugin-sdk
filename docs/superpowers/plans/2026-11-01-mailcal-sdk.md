# Mail and Calendar Provider Plugins in the SDK Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the Plugin SDK a stable mail/calendar provider contract, tooling to build and certify providers, and two reference plugins (Gmail + Google Calendar, Outlook/Microsoft 365) that ActonOS can install from this repo's release.

**Architecture:** Port the ActonOS core's `mail@1`/`calendar@1` ABI and provider kit into sub-packages under `sdk/mailcal/`, byte-compatible with the core. Extend the manifest, `validate`, `MockHost`, and the CLI (`conformance`, `new --type mailcal`, registry `category`). Port the Gmail and Graph backends into `plugins/mail/`, each with a thin `wasip1` `main.go` that is the package `build-all` compiles.

**Tech Stack:** Go 1.26+ (toolchain here is 1.27), wazero v1.12.0, `golang.org/x/net/idna` (new dependency), `GOOS=wasip1 GOARCH=wasm -buildmode=c-shared`.

**Spec:** `docs/superpowers/specs/2026-11-01-mailcal-sdk-design.md`

## Global Constraints

Copied from the spec. Every task implicitly includes them.

- The core's ABI v1 is frozen and additive-only: match it byte for byte, never rename, remove or change the meaning of a field, export or error code. `sdk/mailcal/abi/testdata/golden_v1.json` is copied from the core and `TestGoldenV1` must pass unchanged.
- Plugin ids are `mailcal-gmail` and `mailcal-graph` (same as the core). Directories are `plugins/mail/gmail` and `plugins/mail/outlook`.
- Gmail scopes: `gmail.modify`, `calendar.events`, `calendar.freebusy`. Never `https://mail.google.com/` (permanent delete). Graph scopes: `offline_access Mail.ReadWrite Mail.Send Calendars.ReadWrite`.
- No bare `*` in `net_outbound` or `net_tcp`. Providers do not request `secrets`, `storage`, `workspace` or `bus_events`. `oauth.provider` must equal `permissions.oauth`. A plugin never mixes OAuth and password sign-in.
- Plugins are untrusted code: the core decides whether to send; a plugin sends exactly the `MailSendParams` it receives, and a repeated `IdempotencyKey` must not send twice. `Bcc` is never visible to other recipients.
- `conformance.json` is generated at build time and is **not committed** (compiled output differs between Go versions).
- The `connector-google-workspace` plugin keeps working; only its mail and calendar actions are marked deprecated. Nothing is removed.
- `VERSION` and `sdk/VERSION` become `1.5.0`. `sdk.ABIVersion` (the WASM syscall ABI) stays `1.0.0`.
- Out of scope: IMAP/SMTP, CalDAV, `acton_tcp`, `acton_account`, `icskit`, live Google/Microsoft accounts, any change to the ActonOS repo.

## Review Focus

Input classes the spec implies but no obvious task would exercise. Each has a test in the task named in brackets.

1. A plugin called with no OAuth token, an expired account, or a foreign account must return a coded error (`transient`, `auth_expired`, `invalid`), never crash or hang. [Task 7, Task 8]
2. A malformed or empty request body to any export must return an `invalid` coded error, not a panic. [Task 2]
3. A manifest that asks for a bare `*`, `secrets`, `storage`, `workspace`, `bus_events`, mixes OAuth with credentials, has an `oauth.provider` that does not match `permissions.oauth`, or requests the permanent-delete scope must fail `validate`. [Task 6]
4. A mail plugin's registry entry must carry its capabilities, hash and an explicit category, and its `.actonpkg` must contain the `oauth` block. [Task 10, Task 11 Step 5]
5. A provider that declares an operation it does not implement, or fails to declare one it does, is caught by the conformance suite; and the `new --type mailcal` scaffold must pass that suite untouched. [Task 5, Task 11]

## File Structure

| Path | Responsibility |
|:---|:---|
| `sdk/mailcal/abi/` | Wire types, error codes, export names, address normalisation (`abi.go`, `address.go`, tests, `testdata/golden_v1.json`). Port. |
| `sdk/mailcal/backend.go` | `Backend` interface, `Base`, `Dispatch`. Port of the core's `provkit.go`. |
| `sdk/mailcal/provider.go` | `Provider` alias, `AsError`, `IsCode`, `ErrFunctionNotFound`. New. |
| `sdk/mailcal/bridge.go` | `Caller` + `BridgeProvider` (turns raw export calls into typed calls). Port. |
| `sdk/mailcal/netio/` | `Doer`, `Tokens`, `StatusError`, stdlib implementation for native tests. Port. |
| `sdk/mailcal/guest/` | `wasip1` glue: `Alloc`, `Free`, `Serve`, `HTTP{}`, `Tokens{}`. Port. |
| `sdk/mailcal/mimekit/` | MIME build/parse. Port. |
| `sdk/mailcal/conformance/` | `RunShape` contract suite. Trimmed port. |
| `sdk/mailcal/certificate/` | `conformance.json` read/write/verify. Port. |
| `host/oauth.go`, `host/mailcal.go` | `acton_oauth` mock module; `PluginRunner.CallJSON`. New. |
| `host/testdata/mailprobe/` | Tiny provider used to test the host path end to end. New. |
| `sdk/types.go`, `spec/MANIFEST_SCHEMA.json` | New capabilities, `oauth` block, new permissions. Modify. |
| `cmd/acton-plugin/mailcal_rules.go` | Manifest rules for mail/calendar plugins. New. |
| `cmd/acton-plugin/cmd_conformance.go` | `acton-plugin conformance`. New. |
| `cmd/acton-plugin/cmd_new.go`, `cmd_new_mailcal.go` | `--type mailcal` scaffold. Modify + new. |
| `cmd/acton-plugin/cmd_build_all.go` | Registry `category`. Modify. |
| `plugins/mail/gmail/` | `manifest.json`, `main.go` (wasip1), `internal/gmail/` backend. |
| `plugins/mail/outlook/` | `manifest.json`, `main.go` (wasip1), `internal/graph/` backend. |
| `docs/MAILCAL_PLUGIN_GUIDE.md` and other docs | Guide, changelog, README, built-in plugin list. |

## Porting Helper (used by Tasks 1-5, 8, 9)

Most of the code is copied from the ActonOS core and only its import paths and a few identifiers change. Define this once per shell. It lives outside the repo and is never committed.

```bash
cat > /tmp/port_mailcal.sh <<'EOF'
CORE=/run/media/bieber-kieu/Data/Projects/ActonOS/internal/mailcal
# port <core-file> <sdk-file>: copy with core import paths and identifiers rewritten.
port() {
  mkdir -p "$(dirname "$2")"
  sed -E \
    -e '/"github.com\/actonos\/actonos\/internal\/plugin"$/d' \
    -e 's#github.com/actonos/actonos/internal/mailcal/provkit"#github.com/actonos/plugin-sdk/sdk/mailcal"#' \
    -e 's#github.com/actonos/actonos/internal/mailcal/provkit/wasmguest#github.com/actonos/plugin-sdk/sdk/mailcal/guest#' \
    -e 's#github.com/actonos/actonos/internal/mailcal/provkit/netio#github.com/actonos/plugin-sdk/sdk/mailcal/netio#' \
    -e 's#github.com/actonos/actonos/internal/mailcal/provkit/mimekit#github.com/actonos/plugin-sdk/sdk/mailcal/mimekit#' \
    -e 's#github.com/actonos/actonos/internal/mailcal/plugins/gmail#github.com/actonos/plugin-sdk/plugins/mail/gmail/internal/gmail#' \
    -e 's#github.com/actonos/actonos/internal/mailcal/plugins/graph#github.com/actonos/plugin-sdk/plugins/mail/outlook/internal/graph#' \
    -e 's#github.com/actonos/actonos/internal/mailcal#github.com/actonos/plugin-sdk/sdk/mailcal#' \
    -e 's/\bwasmguest\./guest./g' \
    -e 's/\bprovkit\./mailcal./g' \
    -e 's/\bplugin\.ErrFunctionNotFound/mailcal.ErrFunctionNotFound/g' \
    "$1" \
  | awk '/^\t"github.com\/actonos\/plugin-sdk\/sdk\/mailcal"$/ { if (seen++) next } { print }' > "$2"
}
EOF
source /tmp/port_mailcal.sh
```

The `awk` line drops a duplicate `sdk/mailcal` import (a core file may import both `mailcal` and `provkit`, which both map to `sdk/mailcal`). If `/tmp/port_mailcal.sh` is missing in a later task, recreate it from this block. After any port run `gofmt -w` on the result.

---

### Task 1: Dependency and the mailcal ABI package

**Files:**
- Create: `sdk/mailcal/abi/abi.go`, `sdk/mailcal/abi/address.go`, `sdk/mailcal/abi/abi_test.go`, `sdk/mailcal/abi/address_test.go`, `sdk/mailcal/abi/testdata/golden_v1.json`
- Modify: `go.mod`, `go.sum`

**Interfaces:**
- Produces: package `abi` (`github.com/actonos/plugin-sdk/sdk/mailcal/abi`) with `CapMail`, `CapCalendar`, `Export*` constants, `AllExports`, `ErrorCode` + `Err*` codes, `Error`, `Errorf`, `AccountRef`, `Request`, `Response`, `Capabilities`, all params/result types, `NormalizeEmail`, `NormalizeAddress(es)`.

- [ ] **Step 1: Record the baseline**

Run: `go build ./... && go vet ./... && go test ./... 2>&1 | tail -25`
Expected: note which tests pass. Any pre-existing failure is recorded here and is not caused by this work. (Tests that compile wasm need the Go toolchain and take a while.)

- [ ] **Step 2: Create the porting helper**

Create `/tmp/port_mailcal.sh` from the "Porting Helper" block above and `source` it.

- [ ] **Step 3: Port the ABI package and its golden file**

```bash
source /tmp/port_mailcal.sh
for f in abi.go address.go abi_test.go address_test.go; do port $CORE/abi/$f sdk/mailcal/abi/$f; done
mkdir -p sdk/mailcal/abi/testdata
cp $CORE/abi/testdata/golden_v1.json sdk/mailcal/abi/testdata/golden_v1.json
gofmt -w sdk/mailcal/abi
```

- [ ] **Step 4: Add the dependency**

`address.go` imports `golang.org/x/net/idna`. The module cache already holds `golang.org/x/net v0.57.0`, so work offline first:

```bash
GOFLAGS=-mod=mod GOPROXY=off go get golang.org/x/net@v0.57.0 && GOFLAGS=-mod=mod GOPROXY=off go mod tidy
```
Expected: `go.mod` gains `golang.org/x/net v0.57.0` (and `golang.org/x/text` as indirect). If it fails offline, rerun without `GOPROXY=off`.

- [ ] **Step 5: Run the tests**

Run: `go test ./sdk/mailcal/abi/ -v`
Expected: PASS, including `TestGoldenV1`, `TestUnknownFieldsAreIgnored`, `TestAllExportsAreUniqueAndPrefixed`.

- [ ] **Step 6: Prove it also compiles as WebAssembly**

Run: `GOOS=wasip1 GOARCH=wasm go build ./sdk/mailcal/abi/`
Expected: no output.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum sdk/mailcal/abi
git commit -m "feat(mailcal): add the mail/calendar ABI package ported from the core"
```

---

### Task 2: `Backend`, `Base`, `Dispatch`, `Provider` and the bridge

**Files:**
- Create: `sdk/mailcal/backend.go`, `sdk/mailcal/provider.go`, `sdk/mailcal/bridge.go`, `sdk/mailcal/backend_test.go`

**Interfaces:**
- Consumes: Task 1 `abi` types.
- Produces (package `mailcal`):
  - `type Backend interface { Capabilities, MailList, MailSearch, MailGet, MailGetThread, MailCreateDraft, MailSend, MailMark, MailMove, MailDelete, CalendarList, CalendarGet, CalendarCreate, CalendarUpdate, CalendarRespond, CalendarDelete, CalendarFreeBusy }` (each takes `ctx context.Context, acct abi.AccountRef, ...`, signatures exactly as in the ABI package).
  - `type Base struct{}`: embed it, every operation answers `unsupported`.
  - `func Dispatch(ctx context.Context, b Backend, op string, reqJSON []byte) []byte`: never fails or panics.
  - `type Provider = Backend`; `func AsError(error) *abi.Error`; `func IsCode(error, abi.ErrorCode) bool`; `var ErrFunctionNotFound error`.
  - `type Caller interface { CallJSON(ctx context.Context, export string, req []byte) ([]byte, error) }`; `func NewBridgeProvider(Caller) *BridgeProvider` (implements `Provider`).

- [ ] **Step 1: Write the failing test**

Create `sdk/mailcal/backend_test.go`:

```go
package mailcal_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/actonos/plugin-sdk/sdk/mailcal"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
)

var bg = context.Background()

type panicky struct{ mailcal.Base }

func (panicky) MailList(context.Context, abi.AccountRef, abi.MailListParams) (abi.MailListResult, error) {
	panic("boom")
}

type oneMessage struct{ mailcal.Base }

func (oneMessage) MailList(context.Context, abi.AccountRef, abi.MailListParams) (abi.MailListResult, error) {
	return abi.MailListResult{Messages: []abi.Message{{ID: "m1", Subject: "hello"}}}, nil
}

type dispatchCaller struct{ b mailcal.Backend }

func (c dispatchCaller) CallJSON(ctx context.Context, export string, req []byte) ([]byte, error) {
	return mailcal.Dispatch(ctx, c.b, export, req), nil
}

type missingCaller struct{}

func (missingCaller) CallJSON(context.Context, string, []byte) ([]byte, error) {
	return nil, mailcal.ErrFunctionNotFound
}

func decode(t *testing.T, raw []byte) abi.Response {
	t.Helper()
	var r abi.Response
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("Dispatch returned invalid JSON %q: %v", raw, err)
	}
	return r
}

func TestDispatchTurnsAPanicIntoATransientError(t *testing.T) {
	out := mailcal.Dispatch(bg, panicky{}, abi.ExportMailList, []byte(`{"account":{"id":"a"}}`))
	if r := decode(t, out); r.Error == nil || r.Error.Code != abi.ErrTransient {
		t.Fatalf("want a transient error, got %+v", r)
	}
}

func TestDispatchRejectsMalformedRequestsAsInvalid(t *testing.T) {
	for _, body := range []string{``, `not json`, `[]`, `{"account": 5}`} {
		out := mailcal.Dispatch(bg, oneMessage{}, abi.ExportMailList, []byte(body))
		if r := decode(t, out); r.Error == nil || r.Error.Code != abi.ErrInvalid {
			t.Errorf("body %q: want an invalid error, got %+v", body, r)
		}
	}
}

func TestBaseAnswersUnsupportedForEveryWriteOperation(t *testing.T) {
	for _, op := range []string{abi.ExportMailSend, abi.ExportMailDelete, abi.ExportCalCreateEvent, abi.ExportCalFreeBusy} {
		out := mailcal.Dispatch(bg, mailcal.Base{}, op, []byte(`{"account":{"id":"a"},"params":{}}`))
		if r := decode(t, out); r.Error == nil || r.Error.Code != abi.ErrUnsupported {
			t.Errorf("%s: want unsupported, got %+v", op, r)
		}
	}
}

func TestBridgeRoundTripsATypedCall(t *testing.T) {
	p := mailcal.NewBridgeProvider(dispatchCaller{oneMessage{}})
	res, err := p.MailList(bg, abi.AccountRef{ID: "a"}, abi.MailListParams{})
	if err != nil || len(res.Messages) != 1 || res.Messages[0].Subject != "hello" {
		t.Fatalf("got %+v, %v", res, err)
	}
}

func TestBridgeMapsAMissingExportToUnsupported(t *testing.T) {
	p := mailcal.NewBridgeProvider(missingCaller{})
	_, err := p.MailList(bg, abi.AccountRef{ID: "a"}, abi.MailListParams{})
	if !mailcal.IsCode(err, abi.ErrUnsupported) {
		t.Fatalf("want unsupported, got %v", err)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./sdk/mailcal/ 2>&1 | head`
Expected: FAIL: `no non-test Go files` / undefined `mailcal.Base`.

- [ ] **Step 3: Port the backend and the bridge**

```bash
source /tmp/port_mailcal.sh
port $CORE/provkit/provkit.go sdk/mailcal/backend.go
sed -i 's/^package provkit$/package mailcal/' sdk/mailcal/backend.go
sed -i 's#^// Package provkit is the toolkit#// Package mailcal is the toolkit#' sdk/mailcal/backend.go
port $CORE/bridge.go sdk/mailcal/bridge.go
gofmt -w sdk/mailcal
```

- [ ] **Step 4: Write `provider.go`**

Create `sdk/mailcal/provider.go`:

```go
package mailcal

import (
	"errors"

	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
)

// Provider is the Go view of the mail/calendar ABI. It has exactly the method set of Backend,
// so anything that implements one implements the other.
type Provider = Backend

// ErrFunctionNotFound is what a Caller returns when the plugin does not export the function
// that was asked for. The bridge turns it into an "unsupported" provider error.
var ErrFunctionNotFound = errors.New("plugin function not found")

// AsError extracts the coded provider error from err, or returns nil when err is not one.
func AsError(err error) *abi.Error {
	var e *abi.Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}

// IsCode reports whether err is a provider error with the given code.
func IsCode(err error, code abi.ErrorCode) bool {
	e := AsError(err)
	return e != nil && e.Code == code
}
```

- [ ] **Step 5: Run the tests and the WebAssembly build**

Run: `go test ./sdk/mailcal/ -v && GOOS=wasip1 GOARCH=wasm go build ./sdk/mailcal/`
Expected: all five tests PASS; the wasm build prints nothing. If the compile fails on a name clash (for example a helper declared in both `backend.go` and `bridge.go`), rename the one in `bridge.go`.

- [ ] **Step 6: Commit**

```bash
git add sdk/mailcal
git commit -m "feat(mailcal): add Backend, Base, Dispatch and the call bridge"
```

---

### Task 3: `netio` and `mimekit`

**Files:**
- Create: `sdk/mailcal/netio/netio.go`, `sdk/mailcal/netio/stdlib.go`, `sdk/mailcal/mimekit/mimekit.go`, `sdk/mailcal/mimekit/mimekit_test.go`

**Interfaces:**
- Consumes: Task 1 `abi`.
- Produces: `netio.Request`, `netio.Response` (+ `Header(name)`), `netio.Doer`, `netio.Tokens`, `netio.Login`, `netio.Credentials`, `netio.StatusError(resp)`, `netio.Stdlib{Client}`, `netio.StaticToken`. `mimekit` as in the core (used by the Gmail plugin).

- [ ] **Step 1: Port**

```bash
source /tmp/port_mailcal.sh
port $CORE/provkit/netio/netio.go sdk/mailcal/netio/netio.go
port $CORE/provkit/netio/stdlib.go sdk/mailcal/netio/stdlib.go
port $CORE/provkit/mimekit/mimekit.go sdk/mailcal/mimekit/mimekit.go
port $CORE/provkit/mimekit/mimekit_test.go sdk/mailcal/mimekit/mimekit_test.go
gofmt -w sdk/mailcal
```

- [ ] **Step 2: Run the ported tests**

Run: `go test ./sdk/mailcal/mimekit/ ./sdk/mailcal/netio/ -v`
Expected: `mimekit` tests PASS; `netio` reports no test files.

- [ ] **Step 3: Check both build targets**

Run: `go vet ./sdk/mailcal/... && GOOS=wasip1 GOARCH=wasm go build ./sdk/mailcal/netio/ ./sdk/mailcal/mimekit/`
Expected: no output (`stdlib.go` is excluded from the wasm build by its `!wasip1` tag).

- [ ] **Step 4: Commit**

```bash
git add sdk/mailcal/netio sdk/mailcal/mimekit
git commit -m "feat(mailcal): add netio and mimekit"
```

---

### Task 4: `guest`, the WebAssembly side

**Files:**
- Create: `sdk/mailcal/guest/guest.go`

**Interfaces:**
- Consumes: `abi`, `mailcal.Backend`/`Dispatch`, `netio`.
- Produces (build tag `wasip1`): `guest.Alloc(size uint32) uint32`, `guest.Free(p uint32)`, `guest.Serve(b mailcal.Backend, op string, p, length uint32) uint64`, `guest.HTTP{}` (a `netio.Doer` over `acton_net`), `guest.Tokens{}` (a `netio.Tokens` over `acton_oauth.get_token`, mapping `-3` to `auth_expired`, `-2` to `invalid`, any other negative to `transient`).

- [ ] **Step 1: Port**

`acton_account` and `acton_tcp` are out of scope, so only `wasmguest.go` is ported.

```bash
source /tmp/port_mailcal.sh
port $CORE/provkit/wasmguest/wasmguest.go sdk/mailcal/guest/guest.go
sed -i 's/^package wasmguest$/package guest/' sdk/mailcal/guest/guest.go
sed -i 's#^// Package wasmguest is#// Package guest is#' sdk/mailcal/guest/guest.go
gofmt -w sdk/mailcal/guest
```

- [ ] **Step 2: Fix the doc comment reference**

In `sdk/mailcal/guest/guest.go` the package comment says "see internal/mailcal/refplugin/guest for the pattern". Replace that sentence with: `see plugins/mail/gmail/main.go for the pattern.`

- [ ] **Step 3: Verify**

Run: `GOOS=wasip1 GOARCH=wasm go vet ./sdk/mailcal/guest/ && go build ./... && go vet ./...`
Expected: no output. (The package has a `wasip1` build tag, so native builds skip it. Behaviour is tested through a real `.wasm` in Task 7.)

- [ ] **Step 4: Commit**

```bash
git add sdk/mailcal/guest
git commit -m "feat(mailcal): add the wasip1 guest runtime (Serve, HTTP, Tokens)"
```

---

### Task 5: Conformance suite and certificate

**Files:**
- Create: `sdk/mailcal/conformance/conformance.go`, `sdk/mailcal/conformance/conformance_test.go`, `sdk/mailcal/certificate/certificate.go`, `sdk/mailcal/certificate/certificate_test.go`

**Interfaces:**
- Consumes: `mailcal.Provider`, `mailcal.AsError`, `abi`.
- Produces: `conformance.RunShape(t *testing.T, p mailcal.Provider, acct abi.AccountRef)`; `certificate.Certificate`, `certificate.CertificateFile = "conformance.json"`, `certificate.HashWasm([]byte) string`, `certificate.Verify`, `certificate.Write(path, Certificate) error`, `certificate.Read`, `certificate.VerifyDir`.

The core's suite also has a seeded behaviour suite built on an in-memory reference provider. Gmail and Graph cannot run it, so it is not ported this round.

- [ ] **Step 1: Write the failing test**

Create `sdk/mailcal/conformance/conformance_test.go`:

```go
package conformance_test

import (
	"context"
	"testing"

	"github.com/actonos/plugin-sdk/sdk/mailcal"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
	"github.com/actonos/plugin-sdk/sdk/mailcal/conformance"
)

// readOnly is the smallest valid provider: it lists mail and says "unsupported" to the rest.
type readOnly struct{ mailcal.Base }

func (readOnly) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{
		ABI: []string{abi.CapMail},
		Ops: []string{abi.ExportCapabilities, abi.ExportMailList},
	}, nil
}

func (readOnly) MailList(context.Context, abi.AccountRef, abi.MailListParams) (abi.MailListResult, error) {
	return abi.MailListResult{}, nil
}

type dispatchCaller struct{ b mailcal.Backend }

func (c dispatchCaller) CallJSON(ctx context.Context, export string, req []byte) ([]byte, error) {
	return mailcal.Dispatch(ctx, c.b, export, req), nil
}

func TestShapePassesForAMinimalReadOnlyProvider(t *testing.T) {
	conformance.RunShape(t, mailcal.NewBridgeProvider(dispatchCaller{readOnly{}}), abi.AccountRef{ID: "acct"})
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./sdk/mailcal/conformance/ 2>&1 | head`
Expected: FAIL: no non-test Go files / undefined `conformance.RunShape`.

- [ ] **Step 3: Write `conformance.go`**

Create `sdk/mailcal/conformance/conformance.go`:

```go
// Package conformance is the test suite every mail/calendar provider must pass.
//
// RunShape works against any provider, even a real one pointed at a sandbox account: it
// checks the contract, not the data.
//
// Compatibility rule: cases are only ever added, never loosened. A provider that passed an
// earlier version of this suite must keep passing.
package conformance

import (
	"context"
	"testing"
	"time"

	"github.com/actonos/plugin-sdk/sdk/mailcal"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
)

func ctx() context.Context { return context.Background() }

func requireCode(t *testing.T, err error, code abi.ErrorCode, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected error %q, got success", what, code)
	}
	got := mailcal.AsError(err)
	if got == nil {
		t.Fatalf("%s: expected a coded provider error %q, got plain error: %v", what, code, err)
	}
	if got.Code != code {
		t.Fatalf("%s: error code = %q, want %q (%v)", what, got.Code, code, err)
	}
}

func requireOK(t *testing.T, err error, what string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

// RunShape checks the contract with acct against any provider.
func RunShape(t *testing.T, p mailcal.Provider, acct abi.AccountRef) {
	t.Helper()
	caps, err := p.Capabilities(ctx(), acct)
	requireOK(t, err, "Capabilities")

	if len(caps.ABI) == 0 {
		t.Fatal("Capabilities.ABI is empty; declare mail@1 and/or calendar@1")
	}
	for _, a := range caps.ABI {
		if a != abi.CapMail && a != abi.CapCalendar {
			t.Errorf("unknown ABI entry %q", a)
		}
	}
	known := map[string]bool{}
	for _, name := range abi.AllExports {
		known[name] = true
	}
	for _, op := range caps.Ops {
		if !known[op] {
			t.Errorf("Capabilities.Ops lists %q which is not an ABI export", op)
		}
	}
	if caps.Limits.MaxRecipients < 0 || caps.Limits.MaxBodyBytes < 0 || caps.Limits.MaxAttachmentBytes < 0 {
		t.Errorf("limits must not be negative: %+v", caps.Limits)
	}

	// Each operation either works or says "unsupported"; it never returns an uncoded error.
	type op struct {
		export string
		run    func() error
	}
	now := time.Now().UTC().Truncate(time.Second)
	ops := []op{
		{abi.ExportMailList, func() error { _, e := p.MailList(ctx(), acct, abi.MailListParams{Limit: 1}); return e }},
		{abi.ExportMailSearch, func() error { _, e := p.MailSearch(ctx(), acct, abi.MailSearchParams{Query: "x", Limit: 1}); return e }},
		{abi.ExportMailGet, func() error { _, e := p.MailGet(ctx(), acct, "does-not-exist"); return e }},
		{abi.ExportMailGetThread, func() error { _, e := p.MailGetThread(ctx(), acct, "does-not-exist"); return e }},
		{abi.ExportMailSend, func() error { _, e := p.MailSend(ctx(), acct, abi.MailSendParams{}); return e }},
		{abi.ExportMailMark, func() error { return p.MailMark(ctx(), acct, abi.MailMarkParams{IDs: []string{"does-not-exist"}}) }},
		{abi.ExportMailMove, func() error {
			return p.MailMove(ctx(), acct, abi.MailMoveParams{IDs: []string{"does-not-exist"}, Folder: "Archive"})
		}},
		{abi.ExportMailDelete, func() error { return p.MailDelete(ctx(), acct, abi.MailDeleteParams{IDs: []string{"does-not-exist"}}) }},
		{abi.ExportCalListEvents, func() error {
			_, e := p.CalendarList(ctx(), acct, abi.CalListParams{From: now, To: now.Add(time.Hour)})
			return e
		}},
		{abi.ExportCalGetEvent, func() error { _, e := p.CalendarGet(ctx(), acct, abi.CalGetParams{ID: "does-not-exist"}); return e }},
		{abi.ExportCalDeleteEvent, func() error { return p.CalendarDelete(ctx(), acct, abi.CalDeleteParams{ID: "does-not-exist"}) }},
		{abi.ExportCalFreeBusy, func() error {
			_, e := p.CalendarFreeBusy(ctx(), acct, abi.FreeBusyParams{Emails: []string{"a@example.com"}, From: now, To: now.Add(time.Hour)})
			return e
		}},
	}
	for _, o := range ops {
		o := o
		t.Run("contract/"+o.export, func(t *testing.T) {
			err := o.run()
			if !caps.Supports(o.export) {
				requireCode(t, err, abi.ErrUnsupported, o.export+" not declared")
				return
			}
			// Supported operations with bad or unknown input must fail with a coded error,
			// never succeed silently and never return a transport-level error.
			if err != nil && mailcal.AsError(err) == nil {
				t.Fatalf("%s returned an uncoded error: %v", o.export, err)
			}
		})
	}
}
```

- [ ] **Step 4: Port the certificate package**

```bash
source /tmp/port_mailcal.sh
port $CORE/certificate/certificate.go sdk/mailcal/certificate/certificate.go
port $CORE/certificate/certificate_test.go sdk/mailcal/certificate/certificate_test.go
gofmt -w sdk/mailcal
```

- [ ] **Step 5: Run the tests**

Run: `go test ./sdk/mailcal/... -v 2>&1 | tail -30`
Expected: the new conformance test and the certificate tests PASS, including `TestACertificateNeverContainsSecrets` if the core test of that name exists in the ported file.

- [ ] **Step 6: Commit**

```bash
git add sdk/mailcal/conformance sdk/mailcal/certificate
git commit -m "feat(mailcal): add the shape conformance suite and the certificate package"
```

---

### Task 6: Manifest types, schema and `validate` rules

**Files:**
- Modify: `sdk/types.go` (capabilities, `PluginManifest`, `Permissions`), `spec/MANIFEST_SCHEMA.json`, `cmd/acton-plugin/cmd_validate.go`
- Create: `cmd/acton-plugin/mailcal_rules.go`, `cmd/acton-plugin/mailcal_rules_test.go`

**Interfaces:**
- Produces:
  - `sdk.CapabilityMail Capability = "mail@1"`, `sdk.CapabilityCalendar Capability = "calendar@1"`
  - `sdk.OAuthConfig{Provider, AuthURL, TokenURL string; Scopes []string}`, `PluginManifest.OAuth *OAuthConfig`
  - `Permissions.Workspace bool`, `Permissions.OAuth []string`, `Permissions.Credentials bool`, `Permissions.NetTCP []string`
  - `func validateMailcal(m sdk.PluginManifest) (errs, warns []string)` (pure, package `main`)

- [ ] **Step 1: Write the failing test**

Create `cmd/acton-plugin/mailcal_rules_test.go`:

```go
package main

import (
	"strings"
	"testing"

	"github.com/actonos/plugin-sdk/sdk"
)

func validMail() sdk.PluginManifest {
	return sdk.PluginManifest{
		ID: "mailcal-x", Name: "X", Version: "1.0.0",
		Capabilities: []sdk.Capability{sdk.CapabilityMail, sdk.CapabilityCalendar},
		Permissions:  &sdk.Permissions{NetOutbound: []string{"gmail.googleapis.com"}, OAuth: []string{"google"}},
		OAuth: &sdk.OAuthConfig{
			Provider: "google",
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
			Scopes:   []string{"https://www.googleapis.com/auth/gmail.modify"},
		},
	}
}

func TestAValidMailManifestHasNoErrors(t *testing.T) {
	if errs, _ := validateMailcal(validMail()); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestANonMailPluginIsNotChecked(t *testing.T) {
	m := sdk.PluginManifest{ID: "t", Name: "T", Version: "1.0.0",
		Capabilities: []sdk.Capability{sdk.CapabilityTool},
		Permissions:  &sdk.Permissions{Secrets: []string{"k"}, Storage: true}}
	if errs, _ := validateMailcal(m); len(errs) != 0 {
		t.Fatalf("a tool plugin may use secrets and storage: %v", errs)
	}
}

func TestMailManifestViolationsAreErrors(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*sdk.PluginManifest)
		want   string
	}{
		{"bare wildcard host", func(m *sdk.PluginManifest) { m.Permissions.NetOutbound = []string{"*"} }, "bare '*'"},
		{"bare wildcard tcp", func(m *sdk.PluginManifest) { m.Permissions.NetTCP = []string{"*"} }, "bare '*'"},
		{"tcp without a port", func(m *sdk.PluginManifest) { m.Permissions.NetTCP = []string{"imap.example.com"} }, "host:port"},
		{"secrets", func(m *sdk.PluginManifest) { m.Permissions.Secrets = []string{"k"} }, "secrets"},
		{"storage", func(m *sdk.PluginManifest) { m.Permissions.Storage = true }, "storage"},
		{"workspace", func(m *sdk.PluginManifest) { m.Permissions.Workspace = true }, "workspace"},
		{"bus events", func(m *sdk.PluginManifest) { m.Permissions.BusEvents = []string{"x"} }, "bus_events"},
		{"oauth and credentials", func(m *sdk.PluginManifest) { m.Permissions.Credentials = true }, "both OAuth and password"},
		{"oauth block without permission", func(m *sdk.PluginManifest) { m.Permissions.OAuth = nil }, "permissions.oauth"},
		{"permission without oauth block", func(m *sdk.PluginManifest) { m.OAuth = nil }, "'oauth' block"},
		{"provider mismatch", func(m *sdk.PluginManifest) { m.Permissions.OAuth = []string{"microsoft"} }, "must match"},
		{"two oauth providers", func(m *sdk.PluginManifest) { m.Permissions.OAuth = []string{"google", "microsoft"} }, "must match"},
		{"plain http token url", func(m *sdk.PluginManifest) { m.OAuth.TokenURL = "http://oauth2.googleapis.com/token" }, "https"},
		{"no scopes", func(m *sdk.PluginManifest) { m.OAuth.Scopes = nil }, "scopes"},
		{"permanent delete scope", func(m *sdk.PluginManifest) { m.OAuth.Scopes = []string{"https://mail.google.com/"} }, "permanent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validMail()
			tc.mutate(&m)
			errs, _ := validateMailcal(m)
			if !strings.Contains(strings.Join(errs, "\n"), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, errs)
			}
		})
	}
}

func TestAPasswordProviderIsValid(t *testing.T) {
	m := validMail()
	m.OAuth = nil
	m.Permissions.OAuth = nil
	m.Permissions.Credentials = true
	m.Permissions.NetTCP = []string{"imap.example.com:993"}
	if errs, _ := validateMailcal(m); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/acton-plugin/ 2>&1 | head`
Expected: FAIL: undefined `sdk.CapabilityMail`, `sdk.OAuthConfig`, `validateMailcal`.

- [ ] **Step 3: Extend the SDK types**

In `sdk/types.go` add the two capabilities to the existing const block:

```go
const (
	CapabilityTool      Capability = "tool"
	CapabilityChannel   Capability = "channel"
	CapabilityConnector Capability = "connector"
	// Mail and calendar provider capabilities. The number is the ABI version.
	CapabilityMail     Capability = "mail@1"
	CapabilityCalendar Capability = "calendar@1"
)
```

Add the `OAuth` field to `PluginManifest` (after `Connectors`), and the new permissions to `Permissions`:

```go
	OAuth        *OAuthConfig    `json:"oauth,omitempty"`
```

```go
type Permissions struct {
	NetOutbound []string `json:"net_outbound,omitempty"` // Whitelisted domains (e.g. *.telegram.org, api.github.com)
	Secrets     []string `json:"secrets,omitempty"`      // Hardware Vault keys requested
	Storage     bool     `json:"storage,omitempty"`      // Enable isolated SQLite KV storage
	Workspace   bool     `json:"workspace,omitempty"`    // Enable User Workspace file access
	BusEvents   []string `json:"bus_events,omitempty"`   // Allowed event bus topics
	NetTCP      []string `json:"net_tcp,omitempty"`      // "host:port" TLS endpoints for acton_tcp
	Credentials bool     `json:"credentials,omitempty"`  // May read the username/password of the plugin's own password accounts
	OAuth       []string `json:"oauth,omitempty"`        // OAuth providers whose short-lived access tokens the plugin may request
}

// OAuthConfig tells the core how to sign a user in to a provider plugin's service. The core
// runs the flow and keeps the refresh token; the plugin only receives short-lived access tokens.
type OAuthConfig struct {
	Provider string   `json:"provider"`
	AuthURL  string   `json:"auth_url"`
	TokenURL string   `json:"token_url"`
	Scopes   []string `json:"scopes"`
}
```

- [ ] **Step 4: Write `mailcal_rules.go`**

Create `cmd/acton-plugin/mailcal_rules.go`:

```go
package main

import (
	"fmt"
	"strings"

	"github.com/actonos/plugin-sdk/sdk"
)

// permanentDeleteScope lets a plugin delete a Gmail mailbox's mail for good; providers must not ask for it.
const permanentDeleteScope = "https://mail.google.com/"

func isMailcal(m sdk.PluginManifest) bool {
	for _, c := range m.Capabilities {
		if c == sdk.CapabilityMail || c == sdk.CapabilityCalendar {
			return true
		}
	}
	return false
}

// validateMailcal applies the rules the ActonOS core enforces on mail and calendar provider
// manifests. It returns nothing for any other kind of plugin.
func validateMailcal(m sdk.PluginManifest) (errs, warns []string) {
	if !isMailcal(m) {
		return nil, nil
	}
	p := m.Permissions
	if p == nil {
		p = &sdk.Permissions{}
	}

	for _, h := range p.NetOutbound {
		if strings.TrimSpace(h) == "*" {
			errs = append(errs, "net_outbound must not contain a bare '*'; list the provider's hosts (or '*.domain' when needed)")
		}
	}
	for _, h := range p.NetTCP {
		h = strings.TrimSpace(h)
		switch {
		case h == "*" || strings.HasPrefix(h, "*:"):
			errs = append(errs, "net_tcp must not contain a bare '*'; list 'host:port' endpoints")
		case !strings.Contains(h, ":"):
			errs = append(errs, fmt.Sprintf("net_tcp entry %q must be 'host:port'", h))
		}
	}

	if len(p.Secrets) > 0 {
		errs = append(errs, "mail/calendar providers must not request 'secrets'; the core hands out tokens and passwords itself")
	}
	if p.Storage {
		errs = append(errs, "mail/calendar providers must not request 'storage'")
	}
	if p.Workspace {
		errs = append(errs, "mail/calendar providers must not request 'workspace'")
	}
	if len(p.BusEvents) > 0 {
		errs = append(errs, "mail/calendar providers must not request 'bus_events'")
	}

	if p.Credentials && (m.OAuth != nil || len(p.OAuth) > 0) {
		errs = append(errs, "a plugin must not use both OAuth and password sign-in")
	}

	switch {
	case m.OAuth == nil && len(p.OAuth) > 0:
		errs = append(errs, "permissions.oauth is set but the manifest has no 'oauth' block")
	case m.OAuth != nil:
		o := m.OAuth
		if o.Provider == "" {
			errs = append(errs, "oauth.provider is required")
		}
		if len(p.OAuth) != 1 || p.OAuth[0] != o.Provider {
			errs = append(errs, fmt.Sprintf("permissions.oauth must match oauth.provider: want [%q], got %v", o.Provider, p.OAuth))
		}
		for name, u := range map[string]string{"auth_url": o.AuthURL, "token_url": o.TokenURL} {
			if !strings.HasPrefix(u, "https://") {
				errs = append(errs, fmt.Sprintf("oauth.%s must be an https URL", name))
			}
		}
		if len(o.Scopes) == 0 {
			errs = append(errs, "oauth.scopes must list at least one scope (ask for the narrowest that works)")
		}
		for _, s := range o.Scopes {
			if s == permanentDeleteScope {
				errs = append(errs, "oauth.scopes must not include the permanent-delete scope "+permanentDeleteScope)
			}
		}
	}
	return errs, warns
}
```

- [ ] **Step 5: Wire it into `validate`**

In `cmd/acton-plugin/cmd_validate.go`, in the capability `switch`, add a case before `default`:

```go
		case sdk.CapabilityMail, sdk.CapabilityCalendar:
			// Provider plugins export the mail/calendar ABI instead of listing tools.
```

Immediately after the `// 3. Permissions & SSRF checks` block, add:

```go
	mailErrs, mailWarns := validateMailcal(manifest)
	errors = append(errors, mailErrs...)
	warnings = append(warnings, mailWarns...)
```

Also change the error text of the empty-capabilities check to list the new values: `must declare at least one: 'tool', 'channel', 'connector', 'mail@1', 'calendar@1'`.

- [ ] **Step 6: Update the JSON schema**

In `spec/MANIFEST_SCHEMA.json`:
- In `capabilities.items.enum`, add `"mail@1", "calendar@1"`.
- In `permissions.properties`, add:

```json
        "net_tcp": {
          "type": "array",
          "items": { "type": "string", "pattern": "^[^:]+:[0-9]+$" },
          "description": "TLS endpoints as host:port for acton_tcp (mail/calendar providers)"
        },
        "credentials": {
          "type": "boolean",
          "default": false,
          "description": "May read the username and password of the plugin's own password accounts"
        },
        "oauth": {
          "type": "array",
          "items": { "type": "string" },
          "description": "OAuth providers whose short-lived access tokens the plugin may request (for example google)"
        }
```
- At the top level `properties`, add:

```json
    "oauth": {
      "type": "object",
      "required": ["provider", "auth_url", "token_url", "scopes"],
      "properties": {
        "provider": { "type": "string" },
        "auth_url": { "type": "string", "pattern": "^https://" },
        "token_url": { "type": "string", "pattern": "^https://" },
        "scopes": { "type": "array", "items": { "type": "string" }, "minItems": 1 }
      },
      "description": "How the core signs a user in to a mail/calendar provider. The core keeps the refresh token."
    }
```

- [ ] **Step 7: Run the tests**

Run: `python3 -c "import json;json.load(open('spec/MANIFEST_SCHEMA.json'))" && go test ./cmd/acton-plugin/ ./sdk/ -v 2>&1 | tail -30`
Expected: JSON parses; all new tests PASS; existing `sdk` tests still PASS.

- [ ] **Step 8: Commit**

```bash
git add sdk/types.go spec/MANIFEST_SCHEMA.json cmd/acton-plugin
git commit -m "feat(sdk): add mail/calendar manifest fields and validate rules"
```

---

### Task 7: `MockHost` OAuth module, `CallJSON`, and the probe plugin

**Files:**
- Create: `host/oauth.go`, `host/mailcal.go`, `host/testdata/mailprobe/main.go`, `host/mailcal_test.go`
- Modify: `host/host.go` (struct fields, constructor, module registration, `read_response` return value), `host/runner.go` (`Init`)

**Interfaces:**
- Consumes: Tasks 1-4 (`abi`, `mailcal`, `guest`, `netio`).
- Produces:
  - `(*MockHost).SetOAuthToken(account, token string)`, `.ExpireOAuthAccount(account string)`, `.DenyOAuthAccount(account string)`
  - `(*PluginRunner).CallJSON(ctx context.Context, export string, req []byte) ([]byte, error)`: satisfies `mailcal.Caller`; returns an error wrapping `mailcal.ErrFunctionNotFound` when the export is missing.
  - Return codes identical to the core: `-2` denied, `-3` sign-in expired, `-4` unavailable.

Two existing behaviours must change so the core's guests work: `acton_sys.read_response` must return the number of bytes copied (the guest checks it), and `acton_plugin_init` must be called with as many zero arguments as it declares (mailcal plugins declare two).

- [ ] **Step 1: Write the probe plugin**

Create `host/testdata/mailprobe/main.go`:

```go
//go:build wasip1

// Command mailprobe is a tiny mail provider used to test the host path end to end: it fetches an
// OAuth token, calls an HTTP API with it and reports what it saw in the subject of one message.
package main

import (
	"context"
	"strconv"

	"github.com/actonos/plugin-sdk/sdk/mailcal"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
	"github.com/actonos/plugin-sdk/sdk/mailcal/guest"
	"github.com/actonos/plugin-sdk/sdk/mailcal/netio"
)

func main() {}

type probe struct {
	mailcal.Base
	tokens netio.Tokens
	http   netio.Doer
}

func (probe) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{
		ABI: []string{abi.CapMail},
		Ops: []string{abi.ExportCapabilities, abi.ExportMailList},
	}, nil
}

func (p probe) MailList(ctx context.Context, acct abi.AccountRef, _ abi.MailListParams) (abi.MailListResult, error) {
	tok, err := p.tokens.AccessToken(ctx, acct.ID)
	if err != nil {
		return abi.MailListResult{}, err
	}
	resp, err := p.http.Do(ctx, netio.Request{
		Method:  "GET",
		URL:     "https://api.example.com/probe",
		Headers: map[string]string{"Authorization": "Bearer " + tok},
	})
	if err != nil {
		return abi.MailListResult{}, err
	}
	return abi.MailListResult{Messages: []abi.Message{{ID: "1", Subject: tok + "|" + strconv.Itoa(resp.Status)}}}, nil
}

var backend = probe{tokens: guest.Tokens{}, http: guest.HTTP{}}

//go:wasmexport acton_alloc
func actonAlloc(size uint32) uint32 { return guest.Alloc(size) }

//go:wasmexport acton_free
func actonFree(ptr uint32) { guest.Free(ptr) }

//go:wasmexport acton_plugin_init
func actonPluginInit(ptr, length uint32) uint32 { return 0 }

//go:wasmexport acton_mailcal_capabilities
func capabilities(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportCapabilities, p, l) }

//go:wasmexport acton_mail_list
func mailList(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportMailList, p, l) }
```

- [ ] **Step 2: Write the failing test**

Create `host/mailcal_test.go`:

```go
package host_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/actonos/plugin-sdk/host"
	"github.com/actonos/plugin-sdk/sdk/mailcal"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
)

func buildMailWasm(t *testing.T, pkg string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "plugin.wasm")
	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-o", out, pkg)
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building %s: %v\n%s", pkg, err, b)
	}
	return out
}

func newProbe(t *testing.T) (*host.MockHost, mailcal.Provider) {
	t.Helper()
	ctx := context.Background()
	h, err := host.NewMockHost(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	runner, err := h.LoadPluginFromFile(ctx, buildMailWasm(t, "./testdata/mailprobe"))
	if err != nil {
		t.Fatalf("loading the probe: %v", err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	return h, mailcal.NewBridgeProvider(runner)
}

var mailAcct = abi.AccountRef{ID: "acct-1"}

func TestProbeDeclaresItsCapabilities(t *testing.T) {
	_, p := newProbe(t)
	caps, err := p.Capabilities(context.Background(), mailAcct)
	if err != nil || !caps.Supports(abi.ExportMailList) {
		t.Fatalf("%+v %v", caps, err)
	}
}

func TestProbeUsesTheTokenTheHostGrants(t *testing.T) {
	h, p := newProbe(t)
	h.SetOAuthToken("acct-1", "tok-123")
	h.MockHTTPRoute("https://api.example.com/probe", host.HTTPMockResponse{Status: 200, Body: `{}`})
	res, err := p.MailList(context.Background(), mailAcct, abi.MailListParams{})
	if err != nil || len(res.Messages) != 1 || res.Messages[0].Subject != "tok-123|200" {
		t.Fatalf("got %+v, %v", res, err)
	}
}

func TestProbeMapsEveryOAuthRefusalToACodedError(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*host.MockHost)
		want  abi.ErrorCode
	}{
		{"no token configured", func(*host.MockHost) {}, abi.ErrTransient},
		{"sign-in expired", func(h *host.MockHost) { h.SetOAuthToken("acct-1", "t"); h.ExpireOAuthAccount("acct-1") }, abi.ErrAuthExpired},
		{"account belongs to another plugin", func(h *host.MockHost) { h.SetOAuthToken("acct-1", "t"); h.DenyOAuthAccount("acct-1") }, abi.ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, p := newProbe(t)
			tc.setup(h)
			if _, err := p.MailList(context.Background(), mailAcct, abi.MailListParams{}); !mailcal.IsCode(err, tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestAnExportThePluginLacksIsUnsupported(t *testing.T) {
	_, p := newProbe(t)
	if _, err := p.MailSend(context.Background(), mailAcct, abi.MailSendParams{}); !mailcal.IsCode(err, abi.ErrUnsupported) {
		t.Fatalf("want unsupported, got %v", err)
	}
}
```

- [ ] **Step 3: Run it to verify it fails**

Run: `go test ./host/ -run 'Probe|ExportThePluginLacks' 2>&1 | head` (if a helper name collides with one in the existing `host_test.go`, rename mine)
Expected: FAIL: `h.SetOAuthToken undefined` / `runner` does not satisfy `mailcal.Caller`.

- [ ] **Step 4: Add the OAuth module**

In `host/host.go`, add to the `MockHost` struct:

```go
	oauthTokens      map[string]string
	oauthExpired     map[string]bool
	oauthDenied      map[string]bool
```

In `NewMockHost`, initialise them in the `&MockHost{...}` literal:

```go
		oauthTokens:      make(map[string]string),
		oauthExpired:     make(map[string]bool),
		oauthDenied:      make(map[string]bool),
```

At the end of `registerHostModules`, before its final `return nil`, add:

```go
	if err := h.registerOAuthModule(ctx); err != nil {
		return err
	}
```

Fix `read_response` so it reports the bytes copied (the core does, and `guest` checks it): in the `read_response` function, replace the final `return 0` after the `Write` check with `return int32(writeLen)`.

Create `host/oauth.go`:

```go
package host

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tetratelabs/wazero/api"
)

// Return codes of acton_oauth.get_token, identical to the ActonOS core. A positive value is the
// length of the JSON answer, which the guest then reads with acton_sys.read_response.
const (
	OAuthErrDenied      int32 = -2 // the account is not this plugin's
	OAuthErrAuthExpired int32 = -3 // the user must sign in again
	OAuthErrUnavailable int32 = -4 // no token available (unknown account)
)

// SetOAuthToken makes acton_oauth.get_token return token for account.
func (h *MockHost) SetOAuthToken(account, token string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.oauthTokens[account] = token
}

// ExpireOAuthAccount makes get_token answer "sign in again" for account.
func (h *MockHost) ExpireOAuthAccount(account string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.oauthExpired[account] = true
}

// DenyOAuthAccount makes get_token refuse account, as the core does for another plugin's account.
func (h *MockHost) DenyOAuthAccount(account string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.oauthDenied[account] = true
}

func (h *MockHost) registerOAuthModule(ctx context.Context) error {
	b := h.runtime.NewHostModuleBuilder("acton_oauth")
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, ptr, length uint32) int32 {
			raw, ok := mod.Memory().Read(ptr, length)
			if !ok || len(raw) == 0 || len(raw) > 256 {
				return OAuthErrDenied
			}
			account := string(raw)
			h.mu.RLock()
			denied, expired := h.oauthDenied[account], h.oauthExpired[account]
			token, known := h.oauthTokens[account]
			h.mu.RUnlock()
			switch {
			case denied:
				return OAuthErrDenied
			case expired:
				return OAuthErrAuthExpired
			case !known:
				return OAuthErrUnavailable
			}
			body, _ := json.Marshal(map[string]any{
				"access_token": token,
				"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			})
			return int32(h.setPendingResponse(mod.Name(), body))
		}).
		Export("get_token")
	_, err := b.Instantiate(ctx)
	return err
}
```

- [ ] **Step 5: Make `Init` honour the declared parameter count**

In `host/runner.go`, in `Init`, replace `results, err := initFunc.Call(r.ctx)` with:

```go
		args := make([]uint64, len(initFunc.Definition().ParamTypes()))
		results, err := initFunc.Call(r.ctx, args...)
```

- [ ] **Step 6: Add `CallJSON`**

Create `host/mailcal.go`:

```go
package host

import (
	"context"
	"fmt"

	"github.com/actonos/plugin-sdk/sdk/mailcal"
)

// CallJSON invokes one mail/calendar ABI export with a JSON request and returns the JSON
// response. It satisfies mailcal.Caller, so mailcal.NewBridgeProvider(runner) gives a typed view.
//
// The guest releases the request buffer itself on its next call, so nothing is freed here.
func (r *PluginRunner) CallJSON(ctx context.Context, export string, req []byte) ([]byte, error) {
	fn := r.mod.ExportedFunction(export)
	if fn == nil {
		return nil, fmt.Errorf("%s: %w", export, mailcal.ErrFunctionNotFound)
	}
	ptr, n, err := r.writeBytes(req)
	if err != nil {
		return nil, err
	}
	results, err := fn.Call(ctx, uint64(ptr), uint64(n))
	if err != nil {
		return nil, fmt.Errorf("calling %s: %w", export, err)
	}
	if len(results) == 0 || results[0] == 0 {
		return nil, fmt.Errorf("%s returned no data", export)
	}
	return r.readBytes(uint32(results[0]>>32), uint32(results[0]&0xFFFFFFFF))
}

var _ mailcal.Caller = (*PluginRunner)(nil)
```

- [ ] **Step 7: Run the new tests and the existing host tests**

Run: `go test ./host/ -v 2>&1 | tail -40`
Expected: the five new tests PASS and every existing host test still PASSes (the `read_response` and `Init` changes must not break SDK tool/channel plugins).

- [ ] **Step 8: Commit**

```bash
git add host
git commit -m "feat(host): add the acton_oauth mock module and CallJSON for mail providers"
```

---

### Task 8: Gmail + Google Calendar plugin

**Files:**
- Create: `plugins/mail/gmail/manifest.json`, `plugins/mail/gmail/main.go`, `plugins/mail/gmail/README.md`, `plugins/mail/gmail/internal/gmail/{gmail.go,calendar.go,gmail_test.go,calendar_test.go,fake_test.go}`, `plugins/mail/gmail/wasm_test.go`

**Interfaces:**
- Consumes: Tasks 1-7.
- Produces: package `gmail` (`internal`) with `type Backend struct { mailcal.Base; HTTP netio.Doer; Tokens netio.Tokens; APIBase, CalAPIBase string; ... }`; the plugin `mailcal-gmail`.

- [ ] **Step 1: Port the backend and its tests**

```bash
source /tmp/port_mailcal.sh
for f in gmail.go calendar.go gmail_test.go calendar_test.go fake_test.go; do
  port $CORE/plugins/gmail/$f plugins/mail/gmail/internal/gmail/$f
done
gofmt -w plugins/mail/gmail
go vet ./plugins/mail/gmail/...
```
Expected: `go vet` clean. If it reports an unused or missing import, fix the import block (the port helper drops the core's `internal/plugin` import and merges `provkit`/`mailcal` into one `sdk/mailcal` import).

- [ ] **Step 2: Run the ported tests**

Run: `go test ./plugins/mail/gmail/... -v 2>&1 | tail -40`
Expected: all ported tests PASS, including the idempotent-send, Bcc and `TestGmailPassesTheShapeConformanceSuite`-style tests. These are the behaviour guarantees of the spec; do not weaken or delete any.

- [ ] **Step 3: Write the plugin entry point**

Create `plugins/mail/gmail/main.go`:

```go
//go:build wasip1

// Command gmail is the Gmail and Google Calendar provider plugin (id mailcal-gmail). All behaviour
// lives in internal/gmail; this file only exposes the ABI exports.
package main

import (
	"github.com/actonos/plugin-sdk/plugins/mail/gmail/internal/gmail"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
	"github.com/actonos/plugin-sdk/sdk/mailcal/guest"
)

func main() {}

var backend = &gmail.Backend{HTTP: guest.HTTP{}, Tokens: guest.Tokens{}}

//go:wasmexport acton_alloc
func actonAlloc(size uint32) uint32 { return guest.Alloc(size) }

//go:wasmexport acton_free
func actonFree(ptr uint32) { guest.Free(ptr) }

//go:wasmexport acton_plugin_init
func actonPluginInit(ptr, length uint32) uint32 { return 0 }

//go:wasmexport acton_mailcal_capabilities
func capabilities(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportCapabilities, p, l) }

//go:wasmexport acton_mail_list
func mailList(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportMailList, p, l) }

//go:wasmexport acton_mail_get
func mailGet(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportMailGet, p, l) }

//go:wasmexport acton_mail_get_thread
func mailGetThread(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportMailGetThread, p, l) }

//go:wasmexport acton_mail_search
func mailSearch(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportMailSearch, p, l) }

//go:wasmexport acton_mail_create_draft
func mailCreateDraft(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportMailCreateDraft, p, l) }

//go:wasmexport acton_mail_send
func mailSend(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportMailSend, p, l) }

//go:wasmexport acton_mail_mark
func mailMark(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportMailMark, p, l) }

//go:wasmexport acton_mail_move
func mailMove(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportMailMove, p, l) }

//go:wasmexport acton_mail_delete
func mailDelete(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportMailDelete, p, l) }

//go:wasmexport acton_calendar_list_events
func calList(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportCalListEvents, p, l) }

//go:wasmexport acton_calendar_get_event
func calGet(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportCalGetEvent, p, l) }

//go:wasmexport acton_calendar_create_event
func calCreate(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportCalCreateEvent, p, l) }

//go:wasmexport acton_calendar_update_event
func calUpdate(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportCalUpdateEvent, p, l) }

//go:wasmexport acton_calendar_respond
func calRespond(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportCalRespond, p, l) }

//go:wasmexport acton_calendar_delete_event
func calDelete(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportCalDeleteEvent, p, l) }

//go:wasmexport acton_calendar_free_busy
func calFreeBusy(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportCalFreeBusy, p, l) }
```

Compare the export list with `$CORE/plugins/gmail/wasm/main.go` (`diff <(grep wasmexport main.go) <(grep wasmexport $CORE/plugins/gmail/wasm/main.go)`): the set of exports must be identical.

- [ ] **Step 4: Write the manifest**

Create `plugins/mail/gmail/manifest.json`. Copy the core's manifest and add the registry metadata the other SDK plugins carry:

```bash
cp $CORE/plugins/gmail/manifest.json plugins/mail/gmail/manifest.json
```
Then edit it to add `"author": "ActonOS Core Team"` and `"license": "MIT"` after `"description"`. Expected content otherwise unchanged: `id` `mailcal-gmail`, `capabilities` `["mail@1","calendar@1"]`, `net_outbound` `["gmail.googleapis.com","www.googleapis.com"]`, `permissions.oauth` `["google"]`, and the `oauth` block with exactly the three scopes from the Global Constraints.

- [ ] **Step 5: Write the failing wasm test**

Create `plugins/mail/gmail/wasm_test.go` (package `gmailplugin_test`; the directory's only non-test file is `main.go`, which is `wasip1`-only, so natively it holds just this test. It builds the real `.wasm` and loads it through `MockHost`):

```go
package gmailplugin_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/actonos/plugin-sdk/host"
	"github.com/actonos/plugin-sdk/sdk"
	"github.com/actonos/plugin-sdk/sdk/mailcal"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
	"github.com/actonos/plugin-sdk/sdk/mailcal/conformance"
)

func load(t *testing.T) (*host.MockHost, mailcal.Provider) {
	t.Helper()
	wasm := filepath.Join(t.TempDir(), "plugin.wasm")
	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-o", wasm, ".")
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the plugin: %v\n%s", err, b)
	}
	ctx := context.Background()
	h, err := host.NewMockHost(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	r, err := h.LoadPluginFromFile(ctx, wasm)
	if err != nil {
		t.Fatalf("loading the plugin: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return h, mailcal.NewBridgeProvider(r)
}

func TestManifestAsksForOnlyWhatItNeeds(t *testing.T) {
	raw, err := os.ReadFile("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m sdk.PluginManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.ID != "mailcal-gmail" || m.OAuth == nil || m.OAuth.Provider != "google" ||
		len(m.Permissions.OAuth) != 1 || m.Permissions.OAuth[0] != "google" {
		t.Fatalf("id/oauth block: %q %+v %+v", m.ID, m.OAuth, m.Permissions)
	}
	if len(m.Permissions.Secrets) != 0 || m.Permissions.Storage || m.Permissions.Workspace || len(m.Permissions.BusEvents) != 0 {
		t.Fatalf("a provider needs no secrets, storage, workspace or bus: %+v", m.Permissions)
	}
	for _, s := range m.OAuth.Scopes {
		if s == "https://mail.google.com/" {
			t.Fatal("the full-mailbox scope (permanent delete) must not be requested")
		}
	}
}

func TestCompiledPluginDeclaresMailAndCalendarOperations(t *testing.T) {
	_, p := load(t)
	caps, err := p.Capabilities(context.Background(), abi.AccountRef{ID: "probe"})
	if err != nil || !caps.Supports(abi.ExportMailSend) || !caps.Supports(abi.ExportCalFreeBusy) {
		t.Fatalf("%+v %v", caps, err)
	}
}

// With no token the plugin must fail with a coded error, not crash or hang.
func TestCompiledPluginWithoutATokenFailsWithACodedError(t *testing.T) {
	_, p := load(t)
	if _, err := p.MailList(context.Background(), abi.AccountRef{ID: "probe"}, abi.MailListParams{}); mailcal.AsError(err) == nil {
		t.Fatalf("expected a coded error, got %v", err)
	}
}

func TestCompiledPluginPassesTheOfflineConformanceSuite(t *testing.T) {
	_, p := load(t)
	conformance.RunShape(t, p, abi.AccountRef{ID: "probe"})
}
```

- [ ] **Step 6: Run the wasm tests**

Run: `go test ./plugins/mail/gmail/ -v 2>&1 | tail -30`
Expected: PASS. If `TestCompiledPluginDeclaresMailAndCalendarOperations` fails to load the module, check the `acton_plugin_init` signature and that Task 7 Step 5 was applied.

- [ ] **Step 7: Write the README**

Create `plugins/mail/gmail/README.md` covering: what it does, the scopes, that the user brings their own Google OAuth client, the "Testing" mode 7-day refresh token expiry, and the two unverified assumptions (Gmail keeps the `X-Acton-Idempotency-Key` header; the Gmail API drops `Bcc` before delivery). Reuse the text of the "Gmail and Google Calendar" section of `/run/media/bieber-kieu/Data/Projects/ActonOS/docs/PLUGINS-MAILCAL.md`, replacing the `make plugin-gmail` build step with `acton-plugin build-all --plugin mailcal-gmail`.

- [ ] **Step 8: Validate the manifest and make sure the whole repo still builds**

Run:
```bash
go run ./cmd/acton-plugin validate --manifest plugins/mail/gmail/manifest.json
go build ./... && go vet ./...
```
Expected: `✅ Manifest is valid`; build and vet clean.

- [ ] **Step 9: Commit**

```bash
git add plugins/mail/gmail
git commit -m "feat(plugins): add the Gmail and Google Calendar provider plugin"
```

---

### Task 9: Outlook / Microsoft 365 plugin

**Files:**
- Create: `plugins/mail/outlook/manifest.json`, `plugins/mail/outlook/main.go`, `plugins/mail/outlook/README.md`, `plugins/mail/outlook/internal/graph/{graph.go,calendar.go,graph_test.go,calendar_test.go,calendar_fake_test.go}`, `plugins/mail/outlook/wasm_test.go`

**Interfaces:**
- Consumes: Tasks 1-7.
- Produces: package `graph` (`internal`) with `type Backend struct { mailcal.Base; HTTP netio.Doer; Tokens netio.Tokens; APIBase string }`; the plugin `mailcal-graph`.

- [ ] **Step 1: Port the backend and its tests**

```bash
source /tmp/port_mailcal.sh
for f in graph.go calendar.go graph_test.go calendar_test.go calendar_fake_test.go; do
  port $CORE/plugins/graph/$f plugins/mail/outlook/internal/graph/$f
done
gofmt -w plugins/mail/outlook
go vet ./plugins/mail/outlook/...
go test ./plugins/mail/outlook/... -v 2>&1 | tail -40
```
Expected: vet clean; all ported tests PASS (idempotency by extended property / `transactionId`, pagination cursor must not accept foreign URLs, OData injection in thread ids rejected).

- [ ] **Step 2: Write the plugin entry point**

Create `plugins/mail/outlook/main.go` identical to `plugins/mail/gmail/main.go` (Task 8 Step 3) with three differences: the package comment reads "Command outlook is the Outlook and Microsoft 365 provider plugin (id mailcal-graph)", the import is `"github.com/actonos/plugin-sdk/plugins/mail/outlook/internal/graph"`, and `var backend = &graph.Backend{HTTP: guest.HTTP{}, Tokens: guest.Tokens{}}`. Compare the export set against `$CORE/plugins/graph/wasm/main.go` with `diff <(grep wasmexport ...)`; it must be identical.

- [ ] **Step 3: Write the manifest**

```bash
cp $CORE/plugins/graph/manifest.json plugins/mail/outlook/manifest.json
```
Edit to add `"author": "ActonOS Core Team"` and `"license": "MIT"`. Check it keeps `id` `mailcal-graph`, `oauth.provider` `microsoft`, `permissions.oauth` `["microsoft"]`, the four scopes `offline_access Mail.ReadWrite Mail.Send Calendars.ReadWrite`, and `net_outbound` limited to `graph.microsoft.com`.

- [ ] **Step 4: Write the wasm test**

Create `plugins/mail/outlook/wasm_test.go` as a copy of `plugins/mail/gmail/wasm_test.go` with: the package renamed `outlookplugin_test`, the manifest assertion changed to `m.ID != "mailcal-graph"`, `m.OAuth.Provider != "microsoft"`, `m.Permissions.OAuth[0] != "microsoft"`, and the permanent-delete scope check removed (it is Gmail-specific). Keep the three behavioural tests unchanged.

- [ ] **Step 5: Run, validate, build**

Run:
```bash
go test ./plugins/mail/outlook/ -v 2>&1 | tail -20
go run ./cmd/acton-plugin validate --manifest plugins/mail/outlook/manifest.json
go build ./... && go vet ./...
```
Expected: PASS; manifest valid; clean build.

- [ ] **Step 6: Write the README and commit**

Create `plugins/mail/outlook/README.md` from the "Microsoft 365 and Outlook (Graph)" section of the core's `docs/PLUGINS-MAILCAL.md` (Entra app registration, redirect URI, scopes, search returns one page, delete moves to Deleted Items).

```bash
git add plugins/mail/outlook
git commit -m "feat(plugins): add the Outlook and Microsoft 365 provider plugin"
```

---

### Task 10: Registry `category`, package check, and the `conformance` command

**Files:**
- Modify: `cmd/acton-plugin/cmd_build_all.go`, `cmd/acton-plugin/main.go`
- Create: `cmd/acton-plugin/cmd_conformance.go`, `cmd/acton-plugin/cmd_conformance_test.go`, `cmd/acton-plugin/registry_test.go`

**Interfaces:**
- Consumes: Tasks 5-9.
- Produces:
  - `PluginRegistryEntry.Category string` (`json:"category,omitempty"`), set to `"connector"` for mail/calendar plugins (the marketplace's existing category for third-party service integrations; the core defaults a missing category to `"tool"`).
  - `func categoryFor(caps []sdk.Capability) string`
  - `acton-plugin conformance --wasm <file> --manifest <file> [--account <id>]`: runs `RunShape` offline against the compiled module, writes `conformance.json` next to the `.wasm`, exits non-zero on failure.

- [ ] **Step 1: Write the failing tests**

Create `cmd/acton-plugin/registry_test.go`:

```go
package main

import (
	"testing"

	"github.com/actonos/plugin-sdk/sdk"
)

func TestMailPluginsAreRegisteredAsConnectors(t *testing.T) {
	cases := []struct {
		caps []sdk.Capability
		want string
	}{
		{[]sdk.Capability{sdk.CapabilityMail, sdk.CapabilityCalendar}, "connector"},
		{[]sdk.Capability{sdk.CapabilityCalendar}, "connector"},
		{[]sdk.Capability{sdk.CapabilityChannel}, ""},
		{[]sdk.Capability{sdk.CapabilityTool}, ""},
	}
	for _, c := range cases {
		if got := categoryFor(c.caps); got != c.want {
			t.Errorf("categoryFor(%v) = %q, want %q", c.caps, got, c.want)
		}
	}
}
```

Create `cmd/acton-plugin/cmd_conformance_test.go`:

```go
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/actonos/plugin-sdk/sdk/mailcal/certificate"
)

func TestConformanceCommandCertifiesGmailAndBindsTheCertificateToTheBuild(t *testing.T) {
	dir := t.TempDir()
	wasm := filepath.Join(dir, "plugin.wasm")
	build := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-o", wasm, "../../plugins/mail/gmail")
	build.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building: %v\n%s", err, b)
	}
	if err := runConformance([]string{"--wasm", wasm, "--manifest", "../../plugins/mail/gmail/manifest.json"}); err != nil {
		t.Fatalf("conformance failed: %v", err)
	}
	c, err := certificate.Read(filepath.Join(dir, certificate.CertificateFile))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(wasm)
	if !certificate.Verify(c, raw) || c.PluginID != "mailcal-gmail" || !c.ShapeOK || c.Live {
		t.Fatalf("bad certificate: %+v", c)
	}
	if b, _ := json.Marshal(c); len(b) == 0 {
		t.Fatal("empty certificate")
	}
}

func TestConformanceCommandRejectsAPluginThatIsNotAMailProvider(t *testing.T) {
	err := runConformance([]string{"--wasm", "nope.wasm", "--manifest", "../../plugins/saas/github/manifest.json"})
	if err == nil {
		t.Fatal("a connector plugin must not be certified as a mail provider")
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./cmd/acton-plugin/ 2>&1 | head`
Expected: FAIL: undefined `categoryFor`, `runConformance`.

- [ ] **Step 3: Add `category` to the registry**

In `cmd/acton-plugin/cmd_build_all.go`, add to `PluginRegistryEntry` after `Capabilities`:

```go
	Category     string           `json:"category,omitempty"`
```

Add the helper to `mailcal_rules.go`:

```go
// categoryFor picks the marketplace category for a registry entry. Mail and calendar providers
// are service integrations, so they list under "connector"; for every other plugin the core
// derives the category itself, so the field is left empty.
func categoryFor(caps []sdk.Capability) string {
	for _, c := range caps {
		if c == sdk.CapabilityMail || c == sdk.CapabilityCalendar {
			return "connector"
		}
	}
	return ""
}
```

In the `registryEntries = append(...)` literal in `runBuildAll`, add `Category: categoryFor(manifest.Capabilities),`.

- [ ] **Step 4: Write the `conformance` command**

Create `cmd/acton-plugin/cmd_conformance.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/actonos/plugin-sdk/host"
	"github.com/actonos/plugin-sdk/sdk"
	"github.com/actonos/plugin-sdk/sdk/mailcal"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
	"github.com/actonos/plugin-sdk/sdk/mailcal/certificate"
	"github.com/actonos/plugin-sdk/sdk/mailcal/conformance"
)

// runConformance runs the offline contract suite on a compiled mail/calendar provider and, on
// success, writes conformance.json next to the .wasm. The run needs no token and no network:
// every operation that needs an account must answer with a coded error.
func runConformance(args []string) error {
	fs := flag.NewFlagSet("conformance", flag.ContinueOnError)
	wasmPath := fs.String("wasm", "dist/plugin.wasm", "Path to the compiled .wasm file")
	manifestPath := fs.String("manifest", "manifest.json", "Path to manifest.json")
	account := fs.String("account", "conformance", "Account id used for the probe calls")
	if err := fs.Parse(args); err != nil {
		return err
	}

	raw, err := os.ReadFile(*manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}
	var manifest sdk.PluginManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}
	if !isMailcal(manifest) {
		return fmt.Errorf("%q does not declare mail@1 or calendar@1; conformance only applies to mail/calendar providers", manifest.ID)
	}
	if errs, _ := validateMailcal(manifest); len(errs) > 0 {
		return fmt.Errorf("manifest is not valid for a mail/calendar provider: %v", errs)
	}

	wasm, err := os.ReadFile(*wasmPath)
	if err != nil {
		return fmt.Errorf("reading wasm: %w", err)
	}

	ctx := context.Background()
	mock, err := host.NewMockHost(ctx)
	if err != nil {
		return err
	}
	defer mock.Close()
	mock.SetAllowedDomains(nil) // offline: no network at all
	runner, err := mock.LoadPluginFromBytes(ctx, wasm)
	if err != nil {
		return fmt.Errorf("loading the plugin: %w", err)
	}
	defer runner.Close()
	provider := mailcal.NewBridgeProvider(runner)
	acct := abi.AccountRef{ID: *account}

	caps, capsErr := provider.Capabilities(ctx, acct)
	testing.Init() // RunTests needs the testing flags registered when called outside `go test`
	passed := testing.RunTests(func(pat, str string) (bool, error) { return true, nil }, []testing.InternalTest{{
		Name: "shape",
		F:    func(t *testing.T) { conformance.RunShape(t, provider, acct) },
	}})
	if capsErr != nil || !passed {
		return fmt.Errorf("conformance FAILED for %s (see the output above)", manifest.ID)
	}

	cert := certificate.Certificate{
		PluginID:   manifest.ID,
		Version:    manifest.Version,
		ABI:        caps.ABI,
		Ops:        caps.Ops,
		WasmSHA256: certificate.HashWasm(wasm),
		ShapeOK:    true,
		At:         time.Now().UTC(),
		Tool:       "acton-plugin " + sdk.Version,
	}
	out := filepath.Join(filepath.Dir(*wasmPath), certificate.CertificateFile)
	if err := certificate.Write(out, cert); err != nil {
		return err
	}
	fmt.Printf("OK: %s passed the offline conformance suite -> %s\n", manifest.ID, out)
	return nil
}
```

Wire it in `main.go`: add `case "conformance": err = runConformance(args)` and a usage line `conformance      Run the mail/calendar contract suite on a compiled provider (offline)`.

- [ ] **Step 5: Run the tests**

Run: `go test ./cmd/acton-plugin/ -v 2>&1 | tail -30`
Expected: all PASS. Then check once by hand that the command can fail: temporarily make `plugins/mail/gmail/internal/gmail`'s `Capabilities` list a bogus op such as `"acton_mail_bogus"`, run `go test ./cmd/acton-plugin/ -run Certifies` and expect FAIL; revert the change and rerun to see PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/acton-plugin
git commit -m "feat(cli): add the conformance command and the registry category for mail plugins"
```

---

### Task 11: `new --type mailcal`, `build-all` verification, docs and version

**Files:**
- Modify: `cmd/acton-plugin/cmd_new.go`, `docs/BUILTIN_PLUGINS.md`, `docs/CLI_GUIDE.md`, `README.md`, `CHANGELOG.md`, `plugins/saas/google-workspace/manifest.json`, `plugins/saas/google-workspace/README.md`, `VERSION`, `sdk/VERSION`
- Create: `cmd/acton-plugin/cmd_new_mailcal.go`, `cmd/acton-plugin/cmd_new_mailcal_test.go`, `docs/MAILCAL_PLUGIN_GUIDE.md`

**Interfaces:**
- Consumes: everything above.
- Produces: `acton-plugin new <name> --type=mailcal [--dir D]` writing `manifest.json`, `main.go`, `internal/backend/backend.go`, `internal/backend/backend_test.go`, `README.md`, `.gitignore`.

- [ ] **Step 1: Write the failing scaffold test**

Create `cmd/acton-plugin/cmd_new_mailcal_test.go`. The scaffold must pass the conformance suite untouched (spec criterion 4), so the test generates a plugin inside the SDK module, builds and certifies it:

```go
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMailcalScaffoldBuildsAndPassesConformanceUntouched(t *testing.T) {
	dir := filepath.Join("..", "..", "plugins", "mail", "zz-scaffold-test")
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := runNew([]string{"--type=mailcal", "--dir", dir, "zz-scaffold-test"}); err != nil {
		t.Fatal(err)
	}

	if out, err := exec.Command("go", "test", "./"+filepath.ToSlash(dir)+"/...").CombinedOutput(); err != nil {
		// the scaffold's own tests run the shape suite natively
		t.Fatalf("scaffold tests failed: %v\n%s", err, out)
	}

	wasm := filepath.Join(t.TempDir(), "plugin.wasm")
	build := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-o", wasm, "./"+filepath.ToSlash(dir))
	build.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("scaffold does not compile to wasm: %v\n%s", err, b)
	}
	if err := runValidate([]string{"--manifest", filepath.Join(dir, "manifest.json")}); err != nil {
		t.Fatalf("scaffold manifest invalid: %v", err)
	}
	if err := runConformance([]string{"--wasm", wasm, "--manifest", filepath.Join(dir, "manifest.json")}); err != nil {
		t.Fatalf("scaffold fails conformance: %v", err)
	}
}

func TestNewRejectsAnUnknownType(t *testing.T) {
	if err := runNew([]string{"--type=bogus", "--dir", t.TempDir(), "x"}); err == nil {
		t.Fatal("expected an error for an unknown plugin type")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/acton-plugin/ -run 'Scaffold|RejectsAnUnknown' -v 2>&1 | tail`
Expected: FAIL: `invalid plugin type 'mailcal'`.

- [ ] **Step 3: Write the scaffold generator**

In `cmd/acton-plugin/cmd_new.go`, accept `mailcal` in the type check (error message: `must be 'tool', 'channel', 'connector' or 'mailcal'`) and, immediately after `pluginTypeLower` is validated and the directory created, branch:

```go
	if pluginTypeLower == "mailcal" {
		return scaffoldMailcal(pluginName, targetDir)
	}
```

Create `cmd/acton-plugin/cmd_new_mailcal.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// scaffoldMailcal writes a read-only mail provider that passes the conformance suite as generated.
// It must be created inside this module (for example --dir plugins/mail/<name>) because it
// imports the SDK by its module path, like the other scaffolds.
func scaffoldMailcal(name, dir string) error {
	modulePath, err := moduleImportPath(dir)
	if err != nil {
		return err
	}
	files := map[string]string{
		"manifest.json":                    mailcalManifest(name),
		"main.go":                          mailcalMain(modulePath),
		"internal/backend/backend.go":      mailcalBackend,
		"internal/backend/backend_test.go": mailcalBackendTest(modulePath),
		"README.md":                        fmt.Sprintf("# %s\n\nA mail provider plugin for ActonOS. See docs/MAILCAL_PLUGIN_GUIDE.md.\n", name),
		".gitignore":                       "dist/\n*.wasm\nconformance.json\n",
	}
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("Created mail provider '%s' in '%s'.\n\nNext steps:\n", name, dir)
	fmt.Printf("  acton-plugin validate --manifest %s/manifest.json\n", dir)
	fmt.Printf("  acton-plugin build --source %s --output %s/dist/plugin.wasm\n", dir, dir)
	fmt.Printf("  acton-plugin conformance --wasm %s/dist/plugin.wasm --manifest %s/manifest.json\n", dir, dir)
	return nil
}

func mailcalManifest(name string) string {
	return fmt.Sprintf(`{
  "id": "mailcal-%s",
  "name": "%s",
  "version": "1.0.0",
  "description": "Mail provider for ActonOS.",
  "author": "ActonOS Community",
  "license": "MIT",
  "capabilities": ["mail@1"],
  "permissions": {}
}
`, name, strings.Title(strings.ReplaceAll(name, "-", " ")))
}

func mailcalMain(modulePath string) string {
	return `//go:build wasip1

package main

import (
	"` + modulePath + `/internal/backend"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
	"github.com/actonos/plugin-sdk/sdk/mailcal/guest"
)

func main() {}

var b = backend.Backend{}

//go:wasmexport acton_alloc
func actonAlloc(size uint32) uint32 { return guest.Alloc(size) }

//go:wasmexport acton_free
func actonFree(ptr uint32) { guest.Free(ptr) }

//go:wasmexport acton_plugin_init
func actonPluginInit(ptr, length uint32) uint32 { return 0 }

//go:wasmexport acton_mailcal_capabilities
func capabilities(p, l uint32) uint64 { return guest.Serve(b, abi.ExportCapabilities, p, l) }

//go:wasmexport acton_mail_list
func mailList(p, l uint32) uint64 { return guest.Serve(b, abi.ExportMailList, p, l) }
`
}

const mailcalBackend = `// Package backend is where a provider translates ABI calls into its service's API.
package backend

import (
	"context"

	"github.com/actonos/plugin-sdk/sdk/mailcal"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
)

// Backend embeds mailcal.Base, so every operation you do not override answers "unsupported".
// List exactly the operations you implement in Capabilities().Ops.
type Backend struct{ mailcal.Base }

func (Backend) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{
		ABI: []string{abi.CapMail},
		Ops: []string{abi.ExportCapabilities, abi.ExportMailList},
	}, nil
}

func (Backend) MailList(context.Context, abi.AccountRef, abi.MailListParams) (abi.MailListResult, error) {
	return abi.MailListResult{}, nil
}
`

func mailcalBackendTest(modulePath string) string {
	return `package backend_test

import (
	"context"
	"testing"

	"` + modulePath + `/internal/backend"
	"github.com/actonos/plugin-sdk/sdk/mailcal"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
	"github.com/actonos/plugin-sdk/sdk/mailcal/conformance"
)

type caller struct{ b mailcal.Backend }

func (c caller) CallJSON(ctx context.Context, export string, req []byte) ([]byte, error) {
	return mailcal.Dispatch(ctx, c.b, export, req), nil
}

func TestBackendPassesTheShapeConformanceSuite(t *testing.T) {
	conformance.RunShape(t, mailcal.NewBridgeProvider(caller{backend.Backend{}}), abi.AccountRef{ID: "acct"})
}
`
}

// moduleImportPath returns the Go import path of dir by locating the enclosing go.mod.
func moduleImportPath(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for root := abs; ; root = filepath.Dir(root) {
		raw, err := os.ReadFile(filepath.Join(root, "go.mod"))
		if err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				if mod, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
					rel, _ := filepath.Rel(root, abs)
					return strings.TrimSpace(mod) + "/" + filepath.ToSlash(rel), nil
				}
			}
			return "", fmt.Errorf("no module line in %s", filepath.Join(root, "go.mod"))
		}
		if filepath.Dir(root) == root {
			return "", fmt.Errorf("%s is not inside a Go module; create mail plugins inside the SDK (for example --dir plugins/mail/<name>)", dir)
		}
	}
}
```

After writing the files, run `gofmt -w` over them (`exec.Command("gofmt", "-w", dir).Run()`, ignoring a missing `gofmt`).

- [ ] **Step 4: Run the scaffold test**

Run: `go test ./cmd/acton-plugin/ -run 'Scaffold|RejectsAnUnknown' -v 2>&1 | tail -20`
Expected: PASS. The scaffold's generated test, its `.wasm` build, `validate` and `conformance` all succeed with no edits. The temporary directory `plugins/mail/zz-scaffold-test` is removed by the test; confirm `git status` shows nothing under it.

- [ ] **Step 5: Build everything and check the registry**

Run:
```bash
go run ./cmd/acton-plugin build-all --plugin mailcal
python3 - <<'EOF'
import json, zipfile
reg = json.load(open('dist/plugin-registry.json'))
ids = {p['id']: p for p in reg['plugins']}
for pid in ('mailcal-gmail', 'mailcal-graph'):
    p = ids[pid]
    assert p['capabilities'] == ['mail@1', 'calendar@1'], p['capabilities']
    assert p['category'] == 'connector' and p['sha256'] and p['size_bytes'] > 0, p
    z = zipfile.ZipFile('dist/' + p['filename'])
    m = json.loads(z.read('manifest.json'))
    assert m['oauth']['provider'] in ('google', 'microsoft'), m.get('oauth')
    print('ok', pid, p['size_bytes'])
EOF
```
Expected: both plugins build, and the script prints `ok mailcal-gmail ...` and `ok mailcal-graph ...` (registry entry has capabilities, hash, size, category, and the package carries the `oauth` block). `build-all --plugin` merges into the existing registry, so the other plugins stay listed. `dist/` is git-ignored, so nothing here is committed.

- [ ] **Step 6: Certify the built plugins**

The certificate is written next to the `.wasm`, so certify per-plugin copies to keep the two apart:

```bash
for pair in gmail:mailcal-gmail outlook:mailcal-graph; do
  d=${pair%%:*}; id=${pair##*:}
  mkdir -p /tmp/cert/$id && cp dist/$id.wasm /tmp/cert/$id/plugin.wasm
  go run ./cmd/acton-plugin conformance --wasm /tmp/cert/$id/plugin.wasm --manifest plugins/mail/$d/manifest.json
done
```
Expected: `OK: mailcal-gmail passed ...` and `OK: mailcal-graph passed ...`, with a `conformance.json` in each `/tmp/cert/<id>/` (never committed).

- [ ] **Step 7: Deprecate the old Google Workspace mail and calendar actions**

In `plugins/saas/google-workspace/manifest.json`, change the descriptions of the tools `connector_google-workspace_send_email` and `connector_google-workspace_create_calendar_event` to start with `[Deprecated: use the mailcal-gmail plugin, which goes through ActonOS's approval rules] `, and append the same notice to the plugin `description`. In its `README.md` add a short "Deprecated actions" section naming the two actions and `mailcal-gmail`. Do not change any code or remove any action. Run `go test ./plugins/saas/google-workspace/...` and expect PASS.

- [ ] **Step 8: Write the guide**

Create `docs/MAILCAL_PLUGIN_GUIDE.md` (English), derived from `/run/media/bieber-kieu/Data/Projects/ActonOS/docs/MAILCAL-PLUGIN-GUIDE.md`, with these differences: SDK import paths (`github.com/actonos/plugin-sdk/sdk/mailcal/...`); the scaffold (`acton-plugin new --type=mailcal --dir plugins/mail/my-mail my-mail`; flags must come before the name, because the CLI uses Go's `flag` package) replaces the hand-copied `echo-mail`; build with `acton-plugin build`; test with `acton-plugin conformance`; install via `acton-plugin pack` or the marketplace; a **"Keeping the ABI in sync"** section stating that `sdk/mailcal/abi/testdata/golden_v1.json` is copied from the ActonOS repo (`internal/mailcal/abi/testdata/golden_v1.json`) and must be re-copied whenever the core's ABI gains fields, and that `go test ./sdk/mailcal/abi` then guards the SDK types; and a "What is not verified" section with the two live-account assumptions and the note that marketplace install of `oauth` plugins depends on the core. State that IMAP/SMTP and CalDAV providers are not in the SDK yet.

- [ ] **Step 9: Update the other docs**

- `docs/BUILTIN_PLUGINS.md`: add a "Mail and calendar" section listing `mailcal-gmail` and `mailcal-graph` with their scopes and sign-in model, and mark the two `google-workspace` actions deprecated.
- `docs/CLI_GUIDE.md`: document `conformance` and `new --type=mailcal`.
- `README.md`: add mail/calendar providers to the feature list and link the guide.
- `CHANGELOG.md`: add a `## [1.5.0] - 2026-11-01` entry with Added (mail@1/calendar@1 support in `sdk/mailcal/...`, `acton_oauth` in `MockHost`, `conformance` and `new --type=mailcal`, registry `category`, the two plugins), Changed (`MockHost` `read_response` returns bytes copied; `Init` passes zero arguments to match the declared parameters; manifest schema fields), Deprecated (the two Google Workspace actions). State that the WASM ABI stays `1.0.0` and that IMAP/SMTP and CalDAV are planned.

- [ ] **Step 10: Bump the version**

Run: `./scripts/set_version.sh 1.5.0` (it writes `VERSION` and `sdk/VERSION`; inspect the diff and keep only those two files plus anything the script is documented to sync). Then `go build ./... && go test ./sdk/ -run Version -v` if a version test exists.
Expected: `cat VERSION sdk/VERSION` prints `1.5.0` twice.

- [ ] **Step 11: Full verification**

Run:
```bash
gofmt -l . | grep -v '^dist/' ; go build ./... && go vet ./... && go test ./... 2>&1 | tail -30
```
Expected: `gofmt -l` prints nothing, build/vet are clean, and every package passes, including the pre-existing channel/SaaS tests recorded as passing in Task 1 Step 1.

- [ ] **Step 12: Commit**

```bash
git add cmd docs README.md CHANGELOG.md plugins/saas/google-workspace VERSION sdk/VERSION
git commit -m "feat(cli): add the mailcal scaffold; document mail/calendar plugins; release 1.5.0"
```

---

## Manual Verification (reported, not automated)

Per spec section 9, criterion 5: after `build-all`, install `dist/mailcal-gmail.actonpkg` and `dist/mailcal-graph.actonpkg` into a running ActonOS through its plugin marketplace. Report: whether they appear under "connector", whether install works with an `oauth` block, and whether the account flow can start. If any step needs a core change, write it up for the ActonOS follow-up round and do not patch the core here. The two live-account assumptions (idempotency header kept by Gmail; `Bcc` stripped) also stay open until someone runs a real account.

## Self-Review Notes

- **Spec coverage:** package layout (Tasks 1-5), manifest + `validate` (6), `MockHost` + `CallMailcal` equivalent (7), `new --type mailcal` and `conformance` (10-11), both plugins with wasm tests (8-9), registry/marketplace delivery incl. `category` and `oauth` in the package (10, 11), `google-workspace` deprecation (11), docs and 1.5.0 (11). Criteria 1-6 map to Task 11 Steps 5-6 and 11, Task 9/8 Step 5-6, Task 6, Task 11 Step 4, Task 11 Step 5, and the guide (Step 8).
- **Names used across tasks:** `mailcal.Backend/Base/Dispatch/Provider/Caller/NewBridgeProvider/ErrFunctionNotFound/AsError/IsCode`, `guest.Alloc/Free/Serve/HTTP/Tokens`, `conformance.RunShape`, `certificate.*`, `host.SetOAuthToken/ExpireOAuthAccount/DenyOAuthAccount`, `(*PluginRunner).CallJSON`, `validateMailcal`, `isMailcal`, `categoryFor`, `runConformance`, `scaffoldMailcal`.
- **Known risks flagged in-task:** ported code may need small compile fixes after the import rewrite (Tasks 2, 8, 9); `testing.RunTests` use in the CLI needs the by-hand negative check (Task 10 Step 5).
