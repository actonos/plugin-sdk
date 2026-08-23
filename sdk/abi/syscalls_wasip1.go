//go:build wasip1 || wasm

package abi

// SysLog logs a message to ActonOS system logger.
//
//go:wasmimport acton_sys log
func SysLog(level int32, ptr uint32, length uint32)

// SysReadResponse copies the pending host response into linear memory.
//
//go:wasmimport acton_sys read_response
func SysReadResponse(destPtr uint32, destLen uint32) int32

// NetHTTPRequest performs a sandboxed HTTP request and returns the response byte length.
//
//go:wasmimport acton_net http_request
func NetHTTPRequest(reqPtr uint32, reqLen uint32) uint32

// VaultGetSecret retrieves an authorized secret and returns the secret string byte length.
//
//go:wasmimport acton_vault get_secret
func VaultGetSecret(keyPtr uint32, keyLen uint32) uint32

// StorageKVGet retrieves a value from the plugin's KV store and returns the value byte length.
//
//go:wasmimport acton_storage kv_get
func StorageKVGet(keyPtr uint32, keyLen uint32) uint32

// StorageKVSet stores a key-value pair into the plugin's isolated SQLite KV storage.
//
//go:wasmimport acton_storage kv_set
func StorageKVSet(keyPtr uint32, keyLen uint32, valPtr uint32, valLen uint32) int32

// StorageKVDelete deletes a key from the plugin's isolated SQLite KV storage.
//
//go:wasmimport acton_storage kv_delete
func StorageKVDelete(keyPtr uint32, keyLen uint32) int32

// BusEmitEvent publishes an event onto the ActonOS internal EventBus.
//
//go:wasmimport acton_bus emit_event
func BusEmitEvent(topicPtr uint32, topicLen uint32, payloadPtr uint32, payloadLen uint32) int32
