package conformance_test

import (
	"context"
	"io"
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

// ---- the suite must catch providers that break the contract ----

func verify(p mailcal.Provider) bool {
	return conformance.Verify(io.Discard, p, abi.AccountRef{ID: "acct"})
}

func TestVerifyAcceptsTheMinimalProvider(t *testing.T) {
	if !verify(readOnly{}) {
		t.Fatal("the minimal read-only provider must pass")
	}
}

type noABI struct{ readOnly }

func (noABI) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{Ops: []string{abi.ExportCapabilities}}, nil
}

type bogusOp struct{ readOnly }

func (bogusOp) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{ABI: []string{abi.CapMail}, Ops: []string{abi.ExportCapabilities, "acton_not_an_export"}}, nil
}

// liar declares mail_send but never implements it (Base answers unsupported).
type liar struct{ readOnly }

func (liar) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{ABI: []string{abi.CapMail}, Ops: []string{abi.ExportCapabilities, abi.ExportMailSend}}, nil
}

// shy implements create_draft but does not declare it.
type shy struct{ readOnly }

func (shy) MailCreateDraft(context.Context, abi.AccountRef, abi.MailDraftParams) (abi.MailDraftResult, error) {
	return abi.MailDraftResult{}, abi.Errorf(abi.ErrInvalid, "no recipients")
}

// shyEvent implements create_event, update_event and respond without declaring them.
type shyEvent struct{ readOnly }

func (shyEvent) CalendarCreate(context.Context, abi.AccountRef, abi.CalWriteParams) (abi.Event, error) {
	return abi.Event{}, abi.Errorf(abi.ErrInvalid, "bad event")
}

// sloppy declares mail_send and "succeeds" on an empty message.
type sloppy struct{ readOnly }

func (sloppy) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{ABI: []string{abi.CapMail}, Ops: []string{abi.ExportCapabilities, abi.ExportMailSend}}, nil
}

func (sloppy) MailSend(context.Context, abi.AccountRef, abi.MailSendParams) (abi.MailSendResult, error) {
	return abi.MailSendResult{ProviderMessageID: "x"}, nil
}

func TestVerifyRejectsContractViolations(t *testing.T) {
	cases := map[string]mailcal.Provider{
		"no ABI declared":                        noABI{},
		"an op that is not an ABI export":        bogusOp{},
		"declared but not implemented":           liar{},
		"create_draft implemented, undeclared":   shy{},
		"create_event implemented, undeclared":   shyEvent{},
		"send that succeeds on an empty message": sloppy{},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			if verify(p) {
				t.Fatal("the suite accepted a provider that breaks the contract")
			}
		})
	}
}
