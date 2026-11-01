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
