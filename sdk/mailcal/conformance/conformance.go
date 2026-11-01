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
