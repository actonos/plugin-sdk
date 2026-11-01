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
