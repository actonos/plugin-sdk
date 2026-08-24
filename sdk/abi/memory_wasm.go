//go:build wasip1 || wasm

package abi

import (
	"sync"
	"unsafe"
)

const arenaSize = 8 * 1024 * 1024 // 8MB pre-allocated static arena

var (
	arenaMu     sync.Mutex
	arenaBuf    = make([]byte, arenaSize)
	arenaOffset uint32
	activeCount int
)

// Alloc allocates a block of memory of given size in WASM linear memory from static arena.
//
//go:wasmexport acton_alloc
//export acton_alloc
func Alloc(size uint32) uint32 {
	if size == 0 {
		return 0
	}

	// 8-byte alignment
	alignedSize := (size + 7) & ^uint32(7)
	if alignedSize > arenaSize {
		// Fallback for unusually large single allocation
		buf := make([]byte, size)
		return uint32(uintptr(unsafe.Pointer(&buf[0])))
	}

	arenaMu.Lock()
	defer arenaMu.Unlock()

	if arenaOffset+alignedSize > arenaSize {
		arenaOffset = 0 // Wrap around ring buffer
	}

	ptr := uint32(uintptr(unsafe.Pointer(&arenaBuf[0]))) + arenaOffset
	arenaOffset += alignedSize
	activeCount++

	return ptr
}

// Free releases the allocated buffer reference in the ring arena.
//
//go:wasmexport acton_free
//export acton_free
func Free(ptr uint32, size uint32) {
	if ptr == 0 {
		return
	}

	arenaMu.Lock()
	if activeCount > 0 {
		activeCount--
	}
	if activeCount == 0 {
		arenaOffset = 0
	}
	arenaMu.Unlock()
}

// GetBuffer retrieves a direct slice reference to WASM linear memory.
func GetBuffer(ptr uint32, length uint32) []byte {
	if ptr == 0 || length == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)
}
