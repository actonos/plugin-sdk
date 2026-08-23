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

	name := tool.Name()
	registeredTools[name] = tool
	registeredTools[strings.ToLower(name)] = tool
	registeredTools[strings.ReplaceAll(name, "-", "_")] = tool
	registeredTools[strings.ReplaceAll(name, "_", "-")] = tool

	// Strip prefixes (e.g. connector_github_list_repos -> list_repos, github_list_repos)
	for _, p := range []string{"connector_", "connector-", "tool_", "tool-"} {
		if strings.HasPrefix(name, p) {
			trimmed := strings.TrimPrefix(name, p)
			registeredTools[trimmed] = tool
			registeredTools[strings.ToLower(trimmed)] = tool
			registeredTools[strings.ReplaceAll(trimmed, "-", "_")] = tool
		}
	}
	if parts := strings.Split(name, "_"); len(parts) >= 3 && parts[0] == "connector" {
		// connector_github_list_repos -> list_repos
		actionPart := strings.Join(parts[2:], "_")
		registeredTools[actionPart] = tool
		registeredTools[strings.ToLower(actionPart)] = tool
	}
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
	Name     string          `json:"name,omitempty"`
	Tool     string          `json:"tool,omitempty"`
	Action   string          `json:"action,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
	Params   json.RawMessage `json:"params,omitempty"`
}

// ActonToolExecuteTestWrapper allows invoking acton_tool_execute during unit tests.
func ActonToolExecuteTestWrapper(namePtr uint32, nameLen uint32, argsPtr uint32, argsLen uint32) uint64 {
	return acton_tool_execute(namePtr, nameLen, argsPtr, argsLen)
}

//go:wasmexport acton_tool_execute
//export acton_tool_execute
func acton_tool_execute(namePtr uint32, nameLen uint32, argsPtr uint32, argsLen uint32) (ret uint64) {
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

	pluginMu.RLock()
	defer pluginMu.RUnlock()

	var reqToolName string
	var toolInput []byte

	// Case 1: ActonOS 4-argument call (namePtr, nameLen, argsPtr, argsLen)
	if argsLen > 0 || argsPtr > 0 {
		reqToolName = string(abi.PtrToBytes(namePtr, nameLen))
		toolInput = abi.PtrToBytes(argsPtr, argsLen)
	} else if nameLen > 0 {
		// Case 2: 2-argument fallback call where namePtr contains a JSON envelope or toolName
		raw := abi.PtrToBytes(namePtr, nameLen)
		var envelope ToolExecutionRequest
		if err := json.Unmarshal(raw, &envelope); err == nil && (envelope.ToolName != "" || envelope.Name != "" || envelope.Tool != "" || envelope.Action != "") {
			reqToolName = envelope.ToolName
			if reqToolName == "" {
				reqToolName = envelope.Name
			}
			if reqToolName == "" {
				reqToolName = envelope.Tool
			}
			if reqToolName == "" {
				reqToolName = envelope.Action
			}
			toolInput = envelope.Input
			if len(toolInput) == 0 {
				toolInput = envelope.Params
			}
		} else {
			reqToolName = string(raw)
			toolInput = []byte("{}")
		}
	}

	var targetTool Tool
	if reqToolName != "" {
		if t, ok := registeredTools[reqToolName]; ok {
			targetTool = t
		} else {
			normalized := strings.ToLower(strings.ReplaceAll(reqToolName, "-", "_"))
			if t, ok := registeredTools[normalized]; ok {
				targetTool = t
			} else {
				for k, t := range registeredTools {
					normK := strings.ToLower(strings.ReplaceAll(k, "-", "_"))
					if normK == normalized || strings.HasSuffix(normK, "_"+normalized) || strings.HasSuffix(normalized, "_"+normK) {
						targetTool = t
						break
					}
				}
			}
		}
	}

	// If no envelope or single tool registered, find first matching tool
	if targetTool == nil && len(registeredTools) == 1 {
		for _, t := range registeredTools {
			targetTool = t
			break
		}
	}

	if targetTool == nil {
		defaultCtx.Log().Error("no matching tool registered in plugin", "tool", reqToolName, "registered_count", len(registeredTools))
		res := NewResultError(fmt.Sprintf("no matching tool registered in plugin for: %s", reqToolName))
		b, _ := json.Marshal(res)
		p, l := abi.BytesToPtr(b)
		return abi.PackPtrLen(p, l)
	}

	defaultCtx.Log().Info("Executing tool", "tool", targetTool.Name(), "input", string(toolInput))
	result, err := targetTool.Execute(defaultCtx, toolInput)
	if err != nil {
		defaultCtx.Log().Error("Tool execution failed", "tool", targetTool.Name(), "err", err)
		result = NewResultError(err.Error())
	} else {
		defaultCtx.Log().Info("Tool execution completed successfully", "tool", targetTool.Name())
	}
	if result == nil {
		result = NewResult("success")
	}

	// Guarantee Content is never empty (crucial for LLM function calling like OpenAI Responses API)
	if result.Content == "" {
		if result.Error != "" {
			result.Content = "Error: " + result.Error
		} else if len(result.Data) > 0 {
			if b, err := json.Marshal(result.Data); err == nil {
				result.Content = string(b)
			} else {
				result.Content = "success"
			}
		} else {
			result.Content = "success"
		}
	}

	outBytes, err := json.Marshal(result)
	if err != nil {
		outBytes = []byte(fmt.Sprintf(`{"content":"error","error":"failed to serialize tool result: %s"}`, err.Error()))
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
		defaultCtx.Log().Error("acton_channel_send called but no channel adapter registered")
		return -1
	}

	msgBytes := abi.PtrToBytes(ptr, length)
	var msg OutboundMessage
	if err := json.Unmarshal(msgBytes, &msg); err != nil {
		defaultCtx.Log().Error("failed to unmarshal OutboundMessage", "err", err)
		return -2
	}

	defaultCtx.Log().Info("Channel SendMessage dispatching", "channel", ch.Name(), "recipient", msg.Recipient, "account_id", msg.AccountID)
	if err := ch.SendMessage(defaultCtx, msg); err != nil {
		defaultCtx.Log().Error("channel SendMessage error", "channel", ch.Name(), "recipient", msg.Recipient, "err", err)
		return -3
	}

	defaultCtx.Log().Info("Channel SendMessage completed successfully", "channel", ch.Name(), "recipient", msg.Recipient)
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
		defaultCtx.Log().Error("channel PollMessages error", "channel", ch.Name(), "err", err)
		return 0
	}

	if len(msgs) > 0 {
		defaultCtx.Log().Info("Channel PollMessages received incoming messages", "channel", ch.Name(), "count", len(msgs))
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
			defaultCtx.Log().Error("panic in acton_connector_action", "panic", r)
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
		defaultCtx.Log().Error("acton_connector_action called but no connector registered")
		res := map[string]any{"error": "no connector registered"}
		b, _ := json.Marshal(res)
		p, l := abi.BytesToPtr(b)
		return abi.PackPtrLen(p, l)
	}

	payloadBytes := abi.PtrToBytes(ptr, length)
	var payload ConnectorActionPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		defaultCtx.Log().Error("failed unmarshaling ConnectorActionPayload", "err", err)
		res := map[string]any{"error": fmt.Sprintf("invalid action payload: %v", err)}
		b, _ := json.Marshal(res)
		p, l := abi.BytesToPtr(b)
		return abi.PackPtrLen(p, l)
	}

	defaultCtx.Log().Info("Executing connector action", "connector", conn.Name(), "action", payload.Action)
	output, err := conn.DispatchAction(defaultCtx, payload.Action, payload.Params)
	if err != nil {
		defaultCtx.Log().Error("Connector action failed", "connector", conn.Name(), "action", payload.Action, "err", err)
		res := map[string]any{
			"error":   err.Error(),
			"content": "Error: " + err.Error(),
		}
		b, _ := json.Marshal(res)
		p, l := abi.BytesToPtr(b)
		return abi.PackPtrLen(p, l)
	}

	defaultCtx.Log().Info("Connector action completed successfully", "connector", conn.Name(), "action", payload.Action)
	outBytes, _ := json.Marshal(output)
	if len(outBytes) == 0 || string(outBytes) == "null" {
		outBytes = []byte(`{"status":"success"}`)
	}
	p, l := abi.BytesToPtr(outBytes)
	return abi.PackPtrLen(p, l)
}
