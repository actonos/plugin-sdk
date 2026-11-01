//go:build wasip1

// Command brokenmail is a deliberately non-conforming provider: it lists an operation that is
// not part of the ABI. The CLI must refuse to certify it.
package main

import (
	"context"

	"github.com/actonos/plugin-sdk/sdk/mailcal"
	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
	"github.com/actonos/plugin-sdk/sdk/mailcal/guest"
)

func main() {}

type broken struct{ mailcal.Base }

func (broken) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{
		ABI: []string{abi.CapMail},
		Ops: []string{abi.ExportCapabilities, "acton_not_an_export"},
	}, nil
}

var backend = broken{}

//go:wasmexport acton_alloc
func actonAlloc(size uint32) uint32 { return guest.Alloc(size) }

//go:wasmexport acton_free
func actonFree(ptr uint32) { guest.Free(ptr) }

//go:wasmexport acton_plugin_init
func actonPluginInit(ptr, length uint32) uint32 { return 0 }

//go:wasmexport acton_mailcal_capabilities
func capabilities(p, l uint32) uint64 { return guest.Serve(backend, abi.ExportCapabilities, p, l) }
