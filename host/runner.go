package host

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/actonos/plugin-sdk/sdk"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// PluginRunner manages the execution instance of a WASM plugin inside MockHost.
type PluginRunner struct {
	host    *MockHost
	mod     api.Module
	ctx     context.Context
	wasmRaw []byte
}

// LoadPluginFromFile reads a .wasm binary from disk and instantiates it within MockHost.
func (h *MockHost) LoadPluginFromFile(ctx context.Context, wasmPath string) (*PluginRunner, error) {
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, fmt.Errorf("reading wasm file: %w", err)
	}
	return h.LoadPluginFromBytes(ctx, wasmBytes)
}

// LoadPluginFromBytes instantiates a WASM module from bytecode.
func (h *MockHost) LoadPluginFromBytes(ctx context.Context, wasmBytes []byte) (*PluginRunner, error) {
	// Compile module first
	compiled, err := h.runtime.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("compiling wasm module: %w", err)
	}

	// Configure module: If the module has _initialize or exports, don't let _start terminate the instance
	config := wazero.NewModuleConfig().
		WithStdout(os.Stdout).
		WithStderr(os.Stderr)

	// Check if module exports _initialize or _start
	exports := compiled.ExportedFunctions()
	if _, hasInit := exports["_initialize"]; hasInit {
		config = config.WithStartFunctions("_initialize")
	} else if _, hasStart := exports["_start"]; hasStart {
		// If only _start exists, don't auto-run _start at instantiate time if it exits,
		// or run it without closing
		config = config.WithStartFunctions()
	}

	mod, err := h.runtime.InstantiateModule(ctx, compiled, config)
	if err != nil {
		return nil, fmt.Errorf("instantiating wasm module: %w", err)
	}

	runner := &PluginRunner{
		host:    h,
		mod:     mod,
		ctx:     ctx,
		wasmRaw: wasmBytes,
	}

	if err := runner.Init(); err != nil {
		_ = mod.Close(ctx)
		return nil, fmt.Errorf("plugin init error: %w", err)
	}

	return runner, nil
}

// Init calls the exported acton_plugin_init function.
func (r *PluginRunner) Init() error {
	initFunc := r.mod.ExportedFunction("acton_plugin_init")
	if initFunc != nil {
		results, err := initFunc.Call(r.ctx)
		if err != nil {
			return err
		}
		if len(results) > 0 && int32(results[0]) != 0 {
			return fmt.Errorf("init returned non-zero code: %d", int32(results[0]))
		}
	}
	return nil
}

// SetPluginConfig stores structured configuration into the runner's isolated storage (__config).
func (r *PluginRunner) SetPluginConfig(cfg any) error {
	var configJSON string
	switch c := cfg.(type) {
	case string:
		configJSON = c
	case []byte:
		configJSON = string(c)
	default:
		b, err := json.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("serializing plugin config: %w", err)
		}
		configJSON = string(b)
	}

	r.host.SetStorage("__config", configJSON)
	return nil
}

// ExecuteTool invokes the exported acton_tool_execute with the given tool name and input JSON.
func (r *PluginRunner) ExecuteTool(toolName string, inputJSON []byte) (*sdk.ToolResult, error) {
	execFunc := r.mod.ExportedFunction("acton_tool_execute")
	if execFunc == nil {
		return nil, fmt.Errorf("plugin does not export 'acton_tool_execute'")
	}

	namePtr, nameLen, err := r.writeBytes([]byte(toolName))
	if err != nil {
		return nil, fmt.Errorf("writing toolName into wasm linear memory: %w", err)
	}
	defer r.free(namePtr, nameLen)

	if len(inputJSON) == 0 {
		inputJSON = []byte("{}")
	}
	argsPtr, argsLen, err := r.writeBytes(inputJSON)
	if err != nil {
		return nil, fmt.Errorf("writing input JSON into wasm linear memory: %w", err)
	}
	defer r.free(argsPtr, argsLen)

	var results []uint64
	paramCount := len(execFunc.Definition().ParamTypes())
	if paramCount == 2 {
		envelope := map[string]any{
			"tool_name": toolName,
			"input":     json.RawMessage(inputJSON),
		}
		envBytes, _ := json.Marshal(envelope)
		envPtr, envLen, err := r.writeBytes(envBytes)
		if err != nil {
			return nil, fmt.Errorf("writing envelope into wasm linear memory: %w", err)
		}
		defer r.free(envPtr, envLen)

		results, err = execFunc.Call(r.ctx, uint64(envPtr), uint64(envLen))
		if err != nil {
			return nil, fmt.Errorf("calling acton_tool_execute (2-param): %w", err)
		}
	} else {
		results, err = execFunc.Call(r.ctx, uint64(namePtr), uint64(nameLen), uint64(argsPtr), uint64(argsLen))
		if err != nil {
			return nil, fmt.Errorf("calling acton_tool_execute: %w", err)
		}
	}

	if len(results) == 0 || results[0] == 0 {
		return sdk.NewResult(""), nil
	}

	packed := results[0]
	resPtr := uint32(packed >> 32)
	resLen := uint32(packed & 0xFFFFFFFF)
	defer r.free(resPtr, resLen)

	resBytes, err := r.readBytes(resPtr, resLen)
	if err != nil {
		return nil, fmt.Errorf("reading tool output: %w", err)
	}

	var toolResult sdk.ToolResult
	if err := json.Unmarshal(resBytes, &toolResult); err != nil {
		return nil, fmt.Errorf("parsing tool result JSON: %w (raw: %s)", err, string(resBytes))
	}

	return &toolResult, nil
}

// SendChannelMessage invokes acton_channel_send to transmit an outbound message.
func (r *PluginRunner) SendChannelMessage(msg sdk.OutboundMessage) error {
	sendFunc := r.mod.ExportedFunction("acton_channel_send")
	if sendFunc == nil {
		return fmt.Errorf("plugin does not export 'acton_channel_send'")
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	ptr, length, err := r.writeBytes(msgBytes)
	if err != nil {
		return err
	}
	defer r.free(ptr, length)

	results, err := sendFunc.Call(r.ctx, uint64(ptr), uint64(length))
	if err != nil {
		return err
	}

	if len(results) > 0 && int32(results[0]) != 0 {
		return fmt.Errorf("acton_channel_send failed with code: %d", int32(results[0]))
	}

	return nil
}

// PollChannelMessages invokes acton_channel_poll to retrieve inbound messages.
func (r *PluginRunner) PollChannelMessages() ([]sdk.InboundMessage, error) {
	pollFunc := r.mod.ExportedFunction("acton_channel_poll")
	if pollFunc == nil {
		return nil, fmt.Errorf("plugin does not export 'acton_channel_poll'")
	}

	results, err := pollFunc.Call(r.ctx)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 || results[0] == 0 {
		return nil, nil
	}

	packed := results[0]
	resPtr := uint32(packed >> 32)
	resLen := uint32(packed & 0xFFFFFFFF)
	defer r.free(resPtr, resLen)

	resBytes, err := r.readBytes(resPtr, resLen)
	if err != nil {
		return nil, err
	}

	var messages []sdk.InboundMessage
	if err := json.Unmarshal(resBytes, &messages); err != nil {
		return nil, fmt.Errorf("parsing inbound messages JSON: %w", err)
	}

	return messages, nil
}

// DispatchConnectorAction invokes acton_connector_action for SaaS actions.
func (r *PluginRunner) DispatchConnectorAction(action string, params any) (any, error) {
	actionFunc := r.mod.ExportedFunction("acton_connector_action")
	if actionFunc == nil {
		return nil, fmt.Errorf("plugin does not export 'acton_connector_action'")
	}

	paramBytes, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	payload := sdk.ConnectorActionPayload{
		Action: action,
		Params: paramBytes,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	ptr, length, err := r.writeBytes(payloadBytes)
	if err != nil {
		return nil, err
	}
	defer r.free(ptr, length)

	results, err := actionFunc.Call(r.ctx, uint64(ptr), uint64(length))
	if err != nil {
		return nil, err
	}

	if len(results) == 0 || results[0] == 0 {
		return nil, nil
	}

	packed := results[0]
	resPtr := uint32(packed >> 32)
	resLen := uint32(packed & 0xFFFFFFFF)
	defer r.free(resPtr, resLen)

	resBytes, err := r.readBytes(resPtr, resLen)
	if err != nil {
		return nil, err
	}

	var out any
	if err := json.Unmarshal(resBytes, &out); err != nil {
		return nil, fmt.Errorf("parsing connector output JSON: %w", err)
	}

	return out, nil
}

// Close closes the module instance.
func (r *PluginRunner) Close() error {
	return r.mod.Close(r.ctx)
}

func (r *PluginRunner) writeBytes(data []byte) (uint32, uint32, error) {
	if len(data) == 0 {
		return 0, 0, nil
	}
	size := uint32(len(data))

	allocFunc := r.mod.ExportedFunction("acton_alloc")
	if allocFunc == nil {
		return 0, 0, fmt.Errorf("module missing acton_alloc export")
	}

	results, err := allocFunc.Call(r.ctx, uint64(size))
	if err != nil || len(results) == 0 {
		return 0, 0, fmt.Errorf("acton_alloc failed: %w", err)
	}

	ptr := uint32(results[0])
	if ptr == 0 {
		return 0, 0, fmt.Errorf("acton_alloc returned null pointer")
	}

	if !r.mod.Memory().Write(ptr, data) {
		return 0, 0, fmt.Errorf("failed to write into wasm memory at %d", ptr)
	}

	return ptr, size, nil
}

func (r *PluginRunner) readBytes(ptr uint32, length uint32) ([]byte, error) {
	if ptr == 0 || length == 0 {
		return nil, nil
	}
	data, ok := r.mod.Memory().Read(ptr, length)
	if !ok {
		return nil, fmt.Errorf("failed to read %d bytes from wasm memory at %d", length, ptr)
	}
	buf := make([]byte, length)
	copy(buf, data)
	return buf, nil
}

func (r *PluginRunner) free(ptr uint32, length uint32) {
	if ptr == 0 {
		return
	}
	freeFunc := r.mod.ExportedFunction("acton_free")
	if freeFunc != nil {
		_, _ = freeFunc.Call(r.ctx, uint64(ptr), uint64(length))
	}
}

