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
func mailCreateDraft(p, l uint32) uint64 {
	return guest.Serve(backend, abi.ExportMailCreateDraft, p, l)
}

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
