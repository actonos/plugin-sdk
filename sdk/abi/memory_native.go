//go:build !wasip1 && !wasm

package abi

import "sync"

var (
	allocMu    sync.RWMutex
	allocTable = make(map[uint32][]byte)
	nextHandle uint32 = 1
)

// Alloc allocates a buffer in native host memory with handle table.
func Alloc(size uint32) uint32 {
	if size == 0 {
		return 0
	}
	buf := make([]byte, size)

	allocMu.Lock()
	defer allocMu.Unlock()

	handle := nextHandle
	nextHandle++
	allocTable[handle] = buf
	return handle
}

// Free frees a buffer handle in native memory.
func Free(ptr uint32, size uint32) {
	allocMu.Lock()
	delete(allocTable, ptr)
	allocMu.Unlock()
}

// GetBuffer retrieves the slice associated with a handle.
func GetBuffer(ptr uint32, length uint32) []byte {
	if ptr == 0 || length == 0 {
		return nil
	}

	allocMu.RLock()
	defer allocMu.RUnlock()

	buf, exists := allocTable[ptr]
	if exists {
		if uint32(len(buf)) >= length {
			return buf[:length]
		}
		return buf
	}
	return nil
}
