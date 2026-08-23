//go:build !wasip1 && !wasm

package abi

import (
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
	lastResponse []byte
}

var fallbackHandler = &NativeFallbackHandler{
	KVStore: make(map[string]string),
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
