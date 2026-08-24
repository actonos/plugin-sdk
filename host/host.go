package host

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// MockHost provides a complete simulated ActonOS Host environment with Wazero WebAssembly sandbox.
type MockHost struct {
	ctx              context.Context
	runtime          wazero.Runtime
	mu               sync.RWMutex
	vault            map[string]string
	storage          map[string]string
	busEvents        []BusEvent
	allowedNet       []string
	httpMocks        map[string]HTTPMockResponse
	logs             []LogEntry
	pendingResponses map[string][]byte
	wsConns          map[int32]*mockHostWSConn
	nextWSHandle     int32
}

type mockHostWSConn struct {
	url     string
	headers map[string]string
	queue   [][]byte
	closed  bool
}

// BusEvent records an event emitted by a plugin.
type BusEvent struct {
	Topic   string `json:"topic"`
	Payload []byte `json:"payload"`
}

// LogEntry records a log message emitted by a plugin.
type LogEntry struct {
	Level   int32  `json:"level"`
	Message string `json:"message"`
}

// HTTPMockResponse specifies a pre-configured response for a URL pattern.
type HTTPMockResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

// NewMockHost creates an initialized MockHost.
func NewMockHost(ctx context.Context) (*MockHost, error) {
	r := wazero.NewRuntime(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	h := &MockHost{
		ctx:              ctx,
		runtime:          r,
		vault:            make(map[string]string),
		storage:          make(map[string]string),
		allowedNet:       []string{"*"}, // Default allow in mock
		httpMocks:        make(map[string]HTTPMockResponse),
		pendingResponses: make(map[string][]byte),
		wsConns:          make(map[int32]*mockHostWSConn),
		nextWSHandle:     1,
	}

	if err := h.registerHostModules(ctx); err != nil {
		_ = r.Close(ctx)
		return nil, fmt.Errorf("registering host modules: %w", err)
	}

	return h, nil
}

// Close terminates the Wazero runtime and frees resources.
func (h *MockHost) Close() error {
	return h.runtime.Close(h.ctx)
}

// SetVaultSecret sets a secret in the mock Hardware Vault.
func (h *MockHost) SetVaultSecret(key, value string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.vault[key] = value
}

// SetStorage sets a key-value pair in the mock KV Storage.
func (h *MockHost) SetStorage(key, value string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.storage[key] = value
}

// SetAllowedDomains configures the outbound domain egress whitelist.
func (h *MockHost) SetAllowedDomains(domains []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.allowedNet = domains
}

// MockHTTPRoute configures a mock response for a given URL prefix.
func (h *MockHost) MockHTTPRoute(urlPrefix string, resp HTTPMockResponse) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.httpMocks[urlPrefix] = resp
}

// GetEvents returns all bus events recorded so far.
func (h *MockHost) GetEvents() []BusEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]BusEvent{}, h.busEvents...)
}

// GetLogs returns all log entries recorded so far.
func (h *MockHost) GetLogs() []LogEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]LogEntry{}, h.logs...)
}

func (h *MockHost) registerHostModules(ctx context.Context) error {
	// 1. acton_sys module
	sysBuilder := h.runtime.NewHostModuleBuilder("acton_sys")
	sysBuilder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, level int32, ptr uint32, length uint32) {
			msgBytes, ok := mod.Memory().Read(ptr, length)
			if !ok {
				return
			}
			msg := string(msgBytes)
			h.mu.Lock()
			h.logs = append(h.logs, LogEntry{Level: level, Message: msg})
			h.mu.Unlock()
			slog.Debug("[MockHost:Log]", "level", level, "msg", msg)
		}).
		Export("log")

	sysBuilder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, destPtr uint32, destLen uint32) int32 {
			h.mu.Lock()
			data, exists := h.pendingResponses[mod.Name()]
			h.mu.Unlock()

			if !exists || len(data) == 0 {
				return 0
			}

			writeLen := destLen
			if uint32(len(data)) < writeLen {
				writeLen = uint32(len(data))
			}

			if !mod.Memory().Write(destPtr, data[:writeLen]) {
				return -1
			}
			return 0
		}).
		Export("read_response")

	if _, err := sysBuilder.Instantiate(ctx); err != nil {
		return err
	}

	// 2. acton_net module
	netBuilder := h.runtime.NewHostModuleBuilder("acton_net")
	netBuilder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, reqPtr uint32, reqLen uint32) uint32 {
			reqBytes, ok := mod.Memory().Read(reqPtr, reqLen)
			if !ok {
				return 0
			}

			var req struct {
				Method  string            `json:"method"`
				URL     string            `json:"url"`
				Headers map[string]string `json:"headers"`
				Body    string            `json:"body"`
			}
			if err := json.Unmarshal(reqBytes, &req); err != nil {
				return h.setPendingResponse(mod.Name(), []byte(fmt.Sprintf(`{"status":400,"body":"%s"}`, err.Error())))
			}

			// Validate domain against whitelist
			if !h.isDomainAllowed(req.URL) {
				return h.setPendingResponse(mod.Name(), []byte(`{"status":403,"body":"Domain not permitted in manifest net_outbound"}`))
			}

			// Match mock route
			h.mu.RLock()
			var matchedResp *HTTPMockResponse
			for prefix, mock := range h.httpMocks {
				if strings.HasPrefix(req.URL, prefix) {
					m := mock
					matchedResp = &m
					break
				}
			}
			h.mu.RUnlock()

			if matchedResp != nil {
				respBytes, _ := json.Marshal(matchedResp)
				return h.setPendingResponse(mod.Name(), respBytes)
			}

			// Default fallback mock response
			defaultResp := map[string]any{
				"status":  200,
				"headers": map[string]string{"Content-Type": "application/json"},
				"body":    `{"status":"ok","mock":true}`,
			}
			respBytes, _ := json.Marshal(defaultResp)
			return h.setPendingResponse(mod.Name(), respBytes)
		}).
		Export("http_request")

	if _, err := netBuilder.Instantiate(ctx); err != nil {
		return err
	}

	// 3. acton_vault module
	vaultBuilder := h.runtime.NewHostModuleBuilder("acton_vault")
	vaultBuilder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, keyPtr uint32, keyLen uint32) uint32 {
			keyBytes, ok := mod.Memory().Read(keyPtr, keyLen)
			if !ok {
				return 0
			}
			key := string(keyBytes)

			h.mu.RLock()
			val, exists := h.vault[key]
			h.mu.RUnlock()

			if !exists {
				return 0
			}
			return h.setPendingResponse(mod.Name(), []byte(val))
		}).
		Export("get_secret")

	if _, err := vaultBuilder.Instantiate(ctx); err != nil {
		return err
	}

	// 4. acton_storage module
	storageBuilder := h.runtime.NewHostModuleBuilder("acton_storage")
	storageBuilder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, keyPtr uint32, keyLen uint32) uint32 {
			keyBytes, ok := mod.Memory().Read(keyPtr, keyLen)
			if !ok {
				return 0
			}
			key := string(keyBytes)

			h.mu.RLock()
			val, exists := h.storage[key]
			h.mu.RUnlock()

			if !exists {
				return 0
			}
			return h.setPendingResponse(mod.Name(), []byte(val))
		}).
		Export("kv_get")

	storageBuilder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, keyPtr uint32, keyLen uint32, valPtr uint32, valLen uint32) int32 {
			keyBytes, ok1 := mod.Memory().Read(keyPtr, keyLen)
			valBytes, ok2 := mod.Memory().Read(valPtr, valLen)
			if !ok1 || !ok2 {
				return -1
			}

			h.mu.Lock()
			h.storage[string(keyBytes)] = string(valBytes)
			h.mu.Unlock()
			return 0
		}).
		Export("kv_set")

	storageBuilder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, keyPtr uint32, keyLen uint32) int32 {
			keyBytes, ok := mod.Memory().Read(keyPtr, keyLen)
			if !ok {
				return -1
			}

			h.mu.Lock()
			delete(h.storage, string(keyBytes))
			h.mu.Unlock()
			return 0
		}).
		Export("kv_delete")

	if _, err := storageBuilder.Instantiate(ctx); err != nil {
		return err
	}

	// 5. acton_bus module
	busBuilder := h.runtime.NewHostModuleBuilder("acton_bus")
	busBuilder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, topicPtr uint32, topicLen uint32, payloadPtr uint32, payloadLen uint32) int32 {
			topicBytes, ok1 := mod.Memory().Read(topicPtr, topicLen)
			payloadBytes, ok2 := mod.Memory().Read(payloadPtr, payloadLen)
			if !ok1 || !ok2 {
				return -1
			}

			h.mu.Lock()
			h.busEvents = append(h.busEvents, BusEvent{
				Topic:   string(topicBytes),
				Payload: append([]byte{}, payloadBytes...),
			})
			h.mu.Unlock()
			return 0
		}).
		Export("emit_event")

	if _, err := busBuilder.Instantiate(ctx); err != nil {
		return err
	}

	// 6. acton_ws module
	wsBuilder := h.runtime.NewHostModuleBuilder("acton_ws")
	wsBuilder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, urlPtr, urlLen, headersPtr, headersLen uint32) int32 {
			urlBytes, ok := mod.Memory().Read(urlPtr, urlLen)
			if !ok {
				return -1
			}
			wsURL := string(urlBytes)

			if !h.isDomainAllowed(wsURL) {
				return -1
			}

			var headers map[string]string
			if headersLen > 0 {
				if hBytes, ok := mod.Memory().Read(headersPtr, headersLen); ok {
					_ = json.Unmarshal(hBytes, &headers)
				}
			}

			h.mu.Lock()
			handleID := h.nextWSHandle
			h.nextWSHandle++
			h.wsConns[handleID] = &mockHostWSConn{
				url:     wsURL,
				headers: headers,
				queue:   make([][]byte, 0),
			}
			h.mu.Unlock()

			return handleID
		}).
		Export("ws_connect")

	wsBuilder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, handleID int32, msgType int32, dataPtr, dataLen uint32) int32 {
			h.mu.Lock()
			conn, exists := h.wsConns[handleID]
			h.mu.Unlock()

			if !exists || conn == nil || conn.closed {
				return -1
			}

			dataBytes, ok := mod.Memory().Read(dataPtr, dataLen)
			if !ok {
				return -1
			}
			_ = dataBytes
			return 0
		}).
		Export("ws_send")

	wsBuilder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, handleID int32) int32 {
			h.mu.Lock()
			conn, exists := h.wsConns[handleID]
			if !exists || conn == nil {
				h.mu.Unlock()
				return -1
			}

			if len(conn.queue) == 0 {
				if conn.closed {
					h.mu.Unlock()
					return -1
				}
				h.mu.Unlock()
				return 0
			}

			msg := conn.queue[0]
			conn.queue = conn.queue[1:]
			h.pendingResponses[mod.Name()] = msg
			h.mu.Unlock()

			return int32(len(msg))
		}).
		Export("ws_poll")

	wsBuilder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, handleID int32) int32 {
			h.mu.Lock()
			if conn, exists := h.wsConns[handleID]; exists {
				conn.closed = true
				delete(h.wsConns, handleID)
			}
			h.mu.Unlock()
			return 0
		}).
		Export("ws_close")

	if _, err := wsBuilder.Instantiate(ctx); err != nil {
		return err
	}

	return nil
}

// PushWSMessage simulates an incoming message on a mock WebSocket connection.
func (h *MockHost) PushWSMessage(handleID int32, msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conn, exists := h.wsConns[handleID]; exists && !conn.closed {
		conn.queue = append(conn.queue, msg)
	}
}

func (h *MockHost) setPendingResponse(modName string, data []byte) uint32 {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pendingResponses[modName] = data
	return uint32(len(data))
}

func (h *MockHost) isDomainAllowed(rawURL string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return false
	}

	// Reject localhost, local domain, and private cloud metadata SSRF
	isSSRFTarget := hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") ||
		strings.HasSuffix(hostname, ".local") || hostname == "169.254.169.254"

	if ip := net.ParseIP(hostname); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			isSSRFTarget = true
		}
	}

	for _, pattern := range h.allowedNet {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "*" {
			return !isSSRFTarget
		}
		if pattern == hostname {
			return !isSSRFTarget
		}
		if strings.HasPrefix(pattern, "*.") {
			suffix := strings.TrimPrefix(pattern, "*.")
			if (hostname == suffix || strings.HasSuffix(hostname, "."+suffix)) && !isSSRFTarget {
				return true
			}
		}
	}
	return false
}
