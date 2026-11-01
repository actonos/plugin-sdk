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
