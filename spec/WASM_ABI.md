# ActonOS WASM Plugin ABI Specification

This document defines the low-level Application Binary Interface (ABI) and Memory Protocol between the **ActonOS Host** (powered by Wazero) and **Guest Plugins** compiled to WebAssembly (`wasip1` / `wasm32-wasi`).

---

## 1. Linear Memory & Data Exchange Protocol

WebAssembly runs in an isolated 32-bit linear memory space. Data exchange between Host and Guest uses the **Non-Reentrant Buffer Protocol**:

### 1.1. Host to Guest Inbound Calls (Tools, Channels, Actions)
- Host allocates memory in Guest using `acton_alloc(size)` -> writes payload into Guest linear memory -> calls the exported function `acton_tool_execute(ptr, len)`.
- Guest executes and returns packed 64-bit reference:
  $$\text{PackedResult} = (\text{outPtr} \ll 32) \mid \text{outLen}$$
- Host reads `[outPtr .. outPtr+outLen]` and frees `outPtr` with `acton_free(outPtr, outLen)`.

### 1.2. Guest to Host Syscall Returns (HTTP, Vault, Storage)
- Guest calls a Host Syscall (e.g. `acton_net.http_request(req_ptr, req_len) -> res_len`).
- Host executes the operation and stores the response in a per-instance pending response buffer, returning the byte length `res_len`.
- Guest allocates its local buffer `buf := make([]byte, res_len)` and retrieves the response with `acton_sys.read_response(dest_ptr, res_len)`.
- **Zero Re-entrancy Guarantee**: Prevents re-entrant host calls into guest allocators, ensuring 100% garbage collection stability and zero race conditions.

---

## 2. Guest Exported Functions (Lifecycle & Capabilities)

| Function Name | Signature | Description |
|:---|:---|:---|
| `acton_alloc` | `(size: u32) -> u32` | Allocates linear memory buffer. |
| `acton_free` | `(ptr: u32, size: u32)` | Frees linear memory buffer. |
| `acton_plugin_init` | `() -> i32` | Called upon plugin instantiation. Returns `0` on success. |
| `acton_tool_execute` | `(namePtr: u32, nameLen: u32, argsPtr: u32, argsLen: u32) -> u64` | Executes a Tool with name and input JSON. Returns packed `[ptr:32 \| len:32]` result JSON. |
| `acton_channel_send` | `(ptr: u32, len: u32) -> i32` | Sends an outbound message. Returns status `0` or error code. |
| `acton_channel_poll` | `() -> u64` | Polls incoming channel messages. Returns packed `[ptr:32 \| len:32]`. |
| `acton_connector_action`| `(ptr: u32, len: u32) -> u64` | Dispatches a SaaS action. Returns packed `[ptr:32 \| len:32]`. |

---

## 3. Host Imported Functions (Syscalls)

### 3.1. System & Logging (`acton_sys`)
```wasm
;; Logs a message (level: 1=Debug, 2=Info, 3=Warn, 4=Error)
(import "acton_sys" "log" (func $log (param i32 i32 i32)))

;; Copies the pending response from the last syscall into destination memory
(import "acton_sys" "read_response" (func $read_response (param i32 i32) (result i32)))
```

### 3.2. Outbound Network (`acton_net`)
```wasm
;; Performs an HTTP request (subject to manifest egress whitelist)
;; Returns response byte length (use acton_sys.read_response to read body)
(import "acton_net" "http_request" (func $http_request (param i32 i32) (result i32)))
```

### 3.3. Hardware Vault (`acton_vault`)
```wasm
;; Retrieves a secret from Hardware Vault. Returns byte length.
(import "acton_vault" "get_secret" (func $get_secret (param i32 i32) (result i32)))
```

### 3.4. Key-Value Storage (`acton_storage`)
```wasm
;; Reads a value from SQLite KV store. Returns byte length.
(import "acton_storage" "kv_get" (func $kv_get (param i32 i32) (result i32)))

;; Sets a key-value pair. Returns 0 on success.
(import "acton_storage" "kv_set" (func $kv_set (param i32 i32 i32 i32) (result i32)))

;; Deletes a key. Returns 0 on success.
(import "acton_storage" "kv_delete" (func $kv_delete (param i32 i32) (result i32)))
```

### 3.5. Event Bus (`acton_bus`)
```wasm
;; Emits an event to ActonOS internal EventBus. Returns 0 on success.
(import "acton_bus" "emit_event" (func $emit_event (param i32 i32 i32 i32) (result i32)))
```

### 3.6. Full-Duplex WebSockets (`acton_ws`)
```wasm
;; Connects to a WebSocket server. Returns handle ID (> 0) on success or negative error code.
(import "acton_ws" "ws_connect" (func $ws_connect (param i32 i32) (result i32)))

;; Sends a message payload over an active WebSocket connection. Returns 0 on success.
(import "acton_ws" "ws_send" (func $ws_send (param i32 i32 i32) (result i32)))

;; Polls for incoming WebSocket messages. Returns byte length of message or 0 if queue empty.
(import "acton_ws" "ws_poll" (func $ws_poll (param i32) (result i32)))

;; Closes an active WebSocket connection. Returns 0 on success.
(import "acton_ws" "ws_close" (func $ws_close (param i32) (result i32)))
```
