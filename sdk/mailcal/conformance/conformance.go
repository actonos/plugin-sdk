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
	"io"
	"testing"
	"time"

	"github.com/actonos/plugin-sdk/sdk/mailcal"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
)

func ctx() context.Context { return context.Background() }

func requireCode(t T, err error, code abi.ErrorCode, what string) {
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

func requireOK(t T, err error, what string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

// RunShape checks the contract with acct against any provider, under `go test`.
func RunShape(t *testing.T, p mailcal.Provider, acct abi.AccountRef) {
	t.Helper()
	Check(testingT{t}, p, acct)
}

// Verify runs the same checks outside `go test` (for example from a CLI), prints failures to w,
// and reports whether the provider passed.
func Verify(w io.Writer, p mailcal.Provider, acct abi.AccountRef) bool {
	r := NewReporter(w)
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				if _, isStop := rec.(stop); !isStop {
					panic(rec)
				}
			}
		}()
		Check(r, p, acct)
	}()
	return !r.Failed()
}

// Check is the contract suite. A failing check ends only its own sub-check; a failure of the
// Capabilities call ends the whole run.
func Check(t T, p mailcal.Provider, acct abi.AccountRef) {
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

	// Every operation is probed with unknown ids or invalid input. An operation the provider does
	// not declare must answer "unsupported"; one it declares must not (that would mean it is
	// declared but not implemented) and must fail with a coded error, never an uncoded one. For
	// operations that change something, invalid input must fail: "success" would mean the provider
	// did something with a message or event that is not valid.
	type op struct {
		export   string
		mustFail bool
		run      func() error
	}
	now := time.Now().UTC().Truncate(time.Second)
	ops := []op{
		{abi.ExportMailList, false, func() error { _, e := p.MailList(ctx(), acct, abi.MailListParams{Limit: 1}); return e }},
		{abi.ExportMailSearch, false, func() error {
			_, e := p.MailSearch(ctx(), acct, abi.MailSearchParams{Query: "x", Limit: 1})
			return e
		}},
		{abi.ExportMailGet, false, func() error { _, e := p.MailGet(ctx(), acct, "does-not-exist"); return e }},
		{abi.ExportMailGetThread, false, func() error { _, e := p.MailGetThread(ctx(), acct, "does-not-exist"); return e }},
		{abi.ExportMailCreateDraft, true, func() error { _, e := p.MailCreateDraft(ctx(), acct, abi.MailDraftParams{}); return e }},
		{abi.ExportMailSend, true, func() error { _, e := p.MailSend(ctx(), acct, abi.MailSendParams{}); return e }},
		{abi.ExportMailMark, false, func() error { return p.MailMark(ctx(), acct, abi.MailMarkParams{IDs: []string{"does-not-exist"}}) }},
		{abi.ExportMailMove, false, func() error {
			return p.MailMove(ctx(), acct, abi.MailMoveParams{IDs: []string{"does-not-exist"}, Folder: "Archive"})
		}},
		{abi.ExportMailDelete, false, func() error { return p.MailDelete(ctx(), acct, abi.MailDeleteParams{IDs: []string{"does-not-exist"}}) }},
		{abi.ExportCalListEvents, false, func() error {
			_, e := p.CalendarList(ctx(), acct, abi.CalListParams{From: now, To: now.Add(time.Hour)})
			return e
		}},
		{abi.ExportCalGetEvent, false, func() error { _, e := p.CalendarGet(ctx(), acct, abi.CalGetParams{ID: "does-not-exist"}); return e }},
		{abi.ExportCalCreateEvent, true, func() error { _, e := p.CalendarCreate(ctx(), acct, abi.CalWriteParams{}); return e }},
		{abi.ExportCalUpdateEvent, true, func() error {
			_, e := p.CalendarUpdate(ctx(), acct, abi.CalWriteParams{Event: abi.Event{ID: "does-not-exist"}})
			return e
		}},
		{abi.ExportCalRespond, true, func() error {
			_, e := p.CalendarRespond(ctx(), acct, abi.CalRespondParams{ID: "does-not-exist", Response: "maybe"})
			return e
		}},
		{abi.ExportCalDeleteEvent, false, func() error { return p.CalendarDelete(ctx(), acct, abi.CalDeleteParams{ID: "does-not-exist"}) }},
		{abi.ExportCalFreeBusy, false, func() error {
			_, e := p.CalendarFreeBusy(ctx(), acct, abi.FreeBusyParams{Emails: []string{"a@example.com"}, From: now, To: now.Add(time.Hour)})
			return e
		}},
	}
	for _, o := range ops {
		o := o
		t.Run("contract/"+o.export, func(t T) {
			err := o.run()
			if !caps.Supports(o.export) {
				requireCode(t, err, abi.ErrUnsupported, o.export+" not declared")
				return
			}
			if mailcal.IsCode(err, abi.ErrUnsupported) {
				t.Fatalf("%s is listed in Capabilities.Ops but answers unsupported; implement it or remove it from Ops", o.export)
			}
			if err != nil && mailcal.AsError(err) == nil {
				t.Fatalf("%s returned an uncoded error: %v", o.export, err)
			}
			if err == nil && o.mustFail {
				t.Fatalf("%s accepted invalid input without an error", o.export)
			}
		})
	}
}
