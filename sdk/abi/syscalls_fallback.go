//go:build !wasip1 && !wasm

package abi

import (
	"encoding/json"
	"log/slog"
	"sync"
)

// NativeFallbackHandler allows injecting mock behavior when running natively on Host OS
type NativeFallbackHandler struct {
	mu           sync.RWMutex
	LogFunc      func(level int32, msg string)
	HTTPFunc     func(reqJSON []byte) []byte
	VaultFunc    func(key string) string
	KVStore      map[string]string
	BusEmitFunc  func(topic string, payload []byte) int32
	WSConnectFunc func(url string, headers map[string]string) int32
	WSSendFunc   func(handleID int32, msgType int32, data []byte) int32
	WSPollFunc        func(handleID int32) []byte
	WSCloseFunc       func(handleID int32) int32
	WorkspaceSaveFunc func(reqJSON []byte) []byte
	WorkspaceReadFunc func(reqJSON []byte) []byte
	wsConns           map[int32]*mockWSConn
	nextWSHandle int32
	lastResponse []byte
}

type mockWSConn struct {
	url     string
	headers map[string]string
	queue   [][]byte
	closed  bool
}

var fallbackHandler = &NativeFallbackHandler{
	KVStore:      make(map[string]string),
	wsConns:      make(map[int32]*mockWSConn),
	nextWSHandle: 1,
}

// SetNativeFallbackHandler sets a custom fallback handler for native testing.
func SetNativeFallbackHandler(h *NativeFallbackHandler) {
	fallbackHandler = h
}

func SysLog(level int32, ptr uint32, length uint32) {
	msg := PtrToString(ptr, length)
	if fallbackHandler.LogFunc != nil {
		fallbackHandler.LogFunc(level, msg)
		return
	}
	slog.Info("[HostSysLog Mock]", "level", level, "msg", msg)
}

func SysReadResponse(destPtr uint32, destLen uint32) int32 {
	fallbackHandler.mu.RLock()
	data := fallbackHandler.lastResponse
	fallbackHandler.mu.RUnlock()

	if len(data) == 0 {
		return 0
	}
	buf := GetBuffer(destPtr, destLen)
	if buf != nil {
		copy(buf, data)
	}
	return 0
}

func NetHTTPRequest(reqPtr uint32, reqLen uint32) uint32 {
	reqBytes := PtrToBytes(reqPtr, reqLen)
	var resBytes []byte
	if fallbackHandler.HTTPFunc != nil {
		resBytes = fallbackHandler.HTTPFunc(reqBytes)
	} else {
		resBytes = []byte(`{"status":200,"headers":{"Content-Type":"application/json"},"body":"{}"}`)
	}

	fallbackHandler.mu.Lock()
	fallbackHandler.lastResponse = resBytes
	fallbackHandler.mu.Unlock()

	return uint32(len(resBytes))
}

func VaultGetSecret(keyPtr uint32, keyLen uint32) uint32 {
	key := PtrToString(keyPtr, keyLen)
	var val string
	if fallbackHandler.VaultFunc != nil {
		val = fallbackHandler.VaultFunc(key)
	} else {
		val = "mock_secret_val"
	}

	resBytes := []byte(val)
	fallbackHandler.mu.Lock()
	fallbackHandler.lastResponse = resBytes
	fallbackHandler.mu.Unlock()

	return uint32(len(resBytes))
}

func StorageKVGet(keyPtr uint32, keyLen uint32) uint32 {
	key := PtrToString(keyPtr, keyLen)
	fallbackHandler.mu.RLock()
	val, ok := fallbackHandler.KVStore[key]
	fallbackHandler.mu.RUnlock()

	if !ok {
		return 0
	}

	resBytes := []byte(val)
	fallbackHandler.mu.Lock()
	fallbackHandler.lastResponse = resBytes
	fallbackHandler.mu.Unlock()

	return uint32(len(resBytes))
}

func StorageKVSet(keyPtr uint32, keyLen uint32, valPtr uint32, valLen uint32) int32 {
	key := PtrToString(keyPtr, keyLen)
	val := PtrToString(valPtr, valLen)
	fallbackHandler.mu.Lock()
	fallbackHandler.KVStore[key] = val
	fallbackHandler.mu.Unlock()
	return 0
}

func StorageKVDelete(keyPtr uint32, keyLen uint32) int32 {
	key := PtrToString(keyPtr, keyLen)
	fallbackHandler.mu.Lock()
	delete(fallbackHandler.KVStore, key)
	fallbackHandler.mu.Unlock()
	return 0
}

func BusEmitEvent(topicPtr uint32, topicLen uint32, payloadPtr uint32, payloadLen uint32) int32 {
	topic := PtrToString(topicPtr, topicLen)
	payload := PtrToBytes(payloadPtr, payloadLen)
	if fallbackHandler.BusEmitFunc != nil {
		return fallbackHandler.BusEmitFunc(topic, payload)
	}
	return 0
}

func WSConnect(urlPtr, urlLen, headersPtr, headersLen uint32) int32 {
	url := PtrToString(urlPtr, urlLen)
	var headers map[string]string
	if headersLen > 0 {
		hBytes := PtrToBytes(headersPtr, headersLen)
		_ = json.Unmarshal(hBytes, &headers)
	}

	fallbackHandler.mu.Lock()
	defer fallbackHandler.mu.Unlock()

	if fallbackHandler.WSConnectFunc != nil {
		return fallbackHandler.WSConnectFunc(url, headers)
	}

	handleID := fallbackHandler.nextWSHandle
	fallbackHandler.nextWSHandle++
	fallbackHandler.wsConns[handleID] = &mockWSConn{
		url:     url,
		headers: headers,
		queue:   make([][]byte, 0),
	}
	return handleID
}

func WSSend(handleID int32, msgType int32, dataPtr, dataLen uint32) int32 {
	data := PtrToBytes(dataPtr, dataLen)

	fallbackHandler.mu.Lock()
	defer fallbackHandler.mu.Unlock()

	if fallbackHandler.WSSendFunc != nil {
		return fallbackHandler.WSSendFunc(handleID, msgType, data)
	}

	conn, exists := fallbackHandler.wsConns[handleID]
	if !exists || conn.closed {
		return -1
	}
	return 0
}

func WSPoll(handleID int32) int32 {
	fallbackHandler.mu.Lock()
	defer fallbackHandler.mu.Unlock()

	if fallbackHandler.WSPollFunc != nil {
		msg := fallbackHandler.WSPollFunc(handleID)
		if len(msg) == 0 {
			return 0
		}
		fallbackHandler.lastResponse = msg
		return int32(len(msg))
	}

	conn, exists := fallbackHandler.wsConns[handleID]
	if !exists || conn.closed {
		return -1
	}

	if len(conn.queue) == 0 {
		return 0
	}

	msg := conn.queue[0]
	conn.queue = conn.queue[1:]
	fallbackHandler.lastResponse = msg
	return int32(len(msg))
}

func WSClose(handleID int32) int32 {
	fallbackHandler.mu.Lock()
	defer fallbackHandler.mu.Unlock()

	if fallbackHandler.WSCloseFunc != nil {
		return fallbackHandler.WSCloseFunc(handleID)
	}

	if conn, exists := fallbackHandler.wsConns[handleID]; exists {
		conn.closed = true
		delete(fallbackHandler.wsConns, handleID)
	}
	return 0
}

func WorkspaceSaveFile(reqPtr uint32, reqLen uint32) int32 {
	reqBytes := PtrToBytes(reqPtr, reqLen)
	var resBytes []byte
	if fallbackHandler.WorkspaceSaveFunc != nil {
		resBytes = fallbackHandler.WorkspaceSaveFunc(reqBytes)
	} else {
		var req struct {
			Path     string `json:"path"`
			Name     string `json:"name"`
			MIMEType string `json:"mime_type"`
		}
		_ = json.Unmarshal(reqBytes, &req)
		p := req.Path
		if p == "" {
			p = req.Name
		}
		mime := req.MIMEType
		if mime == "" {
			mime = "application/octet-stream"
		}
		resBytes, _ = json.Marshal(map[string]any{
			"id":         "ws_node_mock",
			"name":       p,
			"path":       p,
			"url":        "/api/workspace/raw?path=" + p,
			"size_bytes": 1024,
			"mime_type":  mime,
		})
	}

	fallbackHandler.mu.Lock()
	fallbackHandler.lastResponse = resBytes
	fallbackHandler.mu.Unlock()

	return int32(len(resBytes))
}

func WorkspaceReadFile(reqPtr uint32, reqLen uint32) int32 {
	reqBytes := PtrToBytes(reqPtr, reqLen)
	var resBytes []byte
	if fallbackHandler.WorkspaceReadFunc != nil {
		resBytes = fallbackHandler.WorkspaceReadFunc(reqBytes)
	} else {
		var req struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(reqBytes, &req)
		resBytes, _ = json.Marshal(map[string]any{
			"id":             "ws_node_mock",
			"name":           req.Path,
			"path":           req.Path,
			"url":            "/api/workspace/raw?path=" + req.Path,
			"size_bytes":     12,
			"mime_type":      "text/plain",
			"content_base64": "bW9jayBjb250ZW50",
		})
	}

	fallbackHandler.mu.Lock()
	fallbackHandler.lastResponse = resBytes
	fallbackHandler.mu.Unlock()

	return int32(len(resBytes))
}

