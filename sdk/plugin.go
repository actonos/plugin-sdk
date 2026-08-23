package sdk

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/actonos/plugin-sdk/sdk/abi"
)

var (
	pluginMu        sync.RWMutex
	registeredTools = make(map[string]Tool)
	activeChannel   ChannelAdapter
	activeConnector Connector
	defaultCtx      = NewContext()
)

// RegisterTool registers a callable Tool with the plugin runtime.
func RegisterTool(tool Tool) {
	pluginMu.Lock()
	defer pluginMu.Unlock()
	registeredTools[tool.Name()] = tool
}

// RegisterChannel registers a chat channel adapter.
func RegisterChannel(channel ChannelAdapter) {
	pluginMu.Lock()
	defer pluginMu.Unlock()
	activeChannel = channel
}

// RegisterConnector registers a SaaS connector.
func RegisterConnector(connector Connector) {
	pluginMu.Lock()
	defer pluginMu.Unlock()
	activeConnector = connector
}

// Serve blocks or sets up the WASM lifecycle.
func Serve() {
	// In WASM execution model, exports handle incoming calls.
	// In native test model, keep process ready.
}

// --- WASM Export Entrypoints ---

//go:wasmexport acton_plugin_init
//export acton_plugin_init
func acton_plugin_init() int32 {
	runtime.Gosched()
	defaultCtx.Log().Info("ActonOS plugin initialized successfully")
	return 0
}

// ToolExecutionRequest represents an envelope for multi-tool execution.
type ToolExecutionRequest struct {
	ToolName string          `json:"tool_name,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
}

// ActonToolExecuteTestWrapper allows invoking acton_tool_execute during unit tests.
func ActonToolExecuteTestWrapper(ptr uint32, length uint32) uint64 {
	return acton_tool_execute(ptr, length)
}

//go:wasmexport acton_tool_execute
//export acton_tool_execute
func acton_tool_execute(ptr uint32, length uint32) (ret uint64) {
	runtime.Gosched()
	defer runtime.Gosched()
	defer func() {
		if r := recover(); r != nil {
			errResult := NewResultError(fmt.Sprintf("plugin panic: %v", r))
			b, _ := json.Marshal(errResult)
			p, l := abi.BytesToPtr(b)
			ret = abi.PackPtrLen(p, l)
		}
	}()

	inputBytes := abi.PtrToBytes(ptr, length)

	pluginMu.RLock()
	defer pluginMu.RUnlock()

	var targetTool Tool
	var toolInput []byte = inputBytes

	// Check if envelope with tool_name was passed
	var envelope ToolExecutionRequest
	if err := json.Unmarshal(inputBytes, &envelope); err == nil && envelope.ToolName != "" {
		if t, ok := registeredTools[envelope.ToolName]; ok {
			targetTool = t
			toolInput = envelope.Input
		} else {
			// Flexible match (dash/underscore tolerance)
			normalizedTarget := strings.ReplaceAll(envelope.ToolName, "-", "_")
			for k, t := range registeredTools {
				if strings.ReplaceAll(k, "-", "_") == normalizedTarget {
					targetTool = t
					toolInput = envelope.Input
					break
				}
			}
		}
	}

	// If no envelope or single tool registered, find first matching tool
	if targetTool == nil {
		if len(registeredTools) == 1 {
			for _, t := range registeredTools {
				targetTool = t
				break
			}
		}
	}

	if targetTool == nil {
		res := NewResultError("no matching tool registered in plugin")
		b, _ := json.Marshal(res)
		p, l := abi.BytesToPtr(b)
		return abi.PackPtrLen(p, l)
	}

	result, err := targetTool.Execute(defaultCtx, toolInput)
	if err != nil {
		result = NewResultError(err.Error())
	}
	if result == nil {
		result = NewResult("")
	}

	outBytes, err := json.Marshal(result)
	if err != nil {
		outBytes = []byte(fmt.Sprintf(`{"error":"failed to serialize tool result: %s"}`, err.Error()))
	}

	outPtr, outLen := abi.BytesToPtr(outBytes)
	return abi.PackPtrLen(outPtr, outLen)
}

//go:wasmexport acton_channel_send
//export acton_channel_send
func acton_channel_send(ptr uint32, length uint32) (ret int32) {
	defer func() {
		if r := recover(); r != nil {
			defaultCtx.Log().Error("panic in acton_channel_send", "panic", r)
			ret = -99
		}
	}()

	pluginMu.RLock()
	ch := activeChannel
	pluginMu.RUnlock()

	if ch == nil {
		return -1
	}

	msgBytes := abi.PtrToBytes(ptr, length)
	var msg OutboundMessage
	if err := json.Unmarshal(msgBytes, &msg); err != nil {
		defaultCtx.Log().Error("failed to unmarshal OutboundMessage", "err", err)
		return -2
	}

	if err := ch.SendMessage(defaultCtx, msg); err != nil {
		defaultCtx.Log().Error("channel SendMessage error", "err", err)
		return -3
	}

	return 0
}

//go:wasmexport acton_channel_poll
//export acton_channel_poll
func acton_channel_poll() (ret uint64) {
	defer func() {
		if r := recover(); r != nil {
			defaultCtx.Log().Error("panic in acton_channel_poll", "panic", r)
			ret = 0
		}
	}()

	pluginMu.RLock()
	ch := activeChannel
	pluginMu.RUnlock()

	if ch == nil {
		return 0
	}

	msgs, err := ch.PollMessages(defaultCtx)
	if err != nil {
		defaultCtx.Log().Error("channel PollMessages error", "err", err)
		return 0
	}

	b, err := json.Marshal(msgs)
	if err != nil {
		return 0
	}

	p, l := abi.BytesToPtr(b)
	return abi.PackPtrLen(p, l)
}

//go:wasmexport acton_connector_action
//export acton_connector_action
func acton_connector_action(ptr uint32, length uint32) (ret uint64) {
	defer func() {
		if r := recover(); r != nil {
			res := map[string]any{"error": fmt.Sprintf("connector panic: %v", r)}
			b, _ := json.Marshal(res)
			p, l := abi.BytesToPtr(b)
			ret = abi.PackPtrLen(p, l)
		}
	}()

	pluginMu.RLock()
	conn := activeConnector
	pluginMu.RUnlock()

	if conn == nil {
		res := map[string]any{"error": "no connector registered"}
		b, _ := json.Marshal(res)
		p, l := abi.BytesToPtr(b)
		return abi.PackPtrLen(p, l)
	}

	payloadBytes := abi.PtrToBytes(ptr, length)
	var payload ConnectorActionPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		res := map[string]any{"error": fmt.Sprintf("invalid action payload: %v", err)}
		b, _ := json.Marshal(res)
		p, l := abi.BytesToPtr(b)
		return abi.PackPtrLen(p, l)
	}

	output, err := conn.DispatchAction(defaultCtx, payload.Action, payload.Params)
	if err != nil {
		res := map[string]any{"error": err.Error()}
		b, _ := json.Marshal(res)
		p, l := abi.BytesToPtr(b)
		return abi.PackPtrLen(p, l)
	}

	outBytes, _ := json.Marshal(output)
	p, l := abi.BytesToPtr(outBytes)
	return abi.PackPtrLen(p, l)
}
