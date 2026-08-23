//go:build wasip1 || wasm

package abi

import (
	"sync"
	"unsafe"
)

// Slab/Pool sizes for fast zero-GC memory allocation
const (
	slabSmall  = 512
	slabMedium = 4096
	slabLarge  = 65536
)

var (
	poolMu       sync.Mutex
	smallPool    [][]byte
	mediumPool   [][]byte
	largePool    [][]byte
	activeBlocks = make(map[uint32][]byte)
)

// Alloc allocates a block of memory of given size in WASM linear memory with zero-GC recycling.
//
//go:wasmexport acton_alloc
//export acton_alloc
func Alloc(size uint32) uint32 {
	if size == 0 {
		return 0
	}

	var buf []byte

	poolMu.Lock()
	if size <= slabSmall && len(smallPool) > 0 {
		buf = smallPool[len(smallPool)-1]
		smallPool = smallPool[:len(smallPool)-1]
	} else if size <= slabMedium && len(mediumPool) > 0 {
		buf = mediumPool[len(mediumPool)-1]
		mediumPool = mediumPool[:len(mediumPool)-1]
	} else if size <= slabLarge && len(largePool) > 0 {
		buf = largePool[len(largePool)-1]
		largePool = largePool[:len(largePool)-1]
	}
	poolMu.Unlock()

	if buf == nil || uint32(len(buf)) < size {
		allocSize := size
		if size <= slabSmall {
			allocSize = slabSmall
		} else if size <= slabMedium {
			allocSize = slabMedium
		} else if size <= slabLarge {
			allocSize = slabLarge
		}
		buf = make([]byte, allocSize)
	}

	ptr := uint32(uintptr(unsafe.Pointer(&buf[0])))

	poolMu.Lock()
	activeBlocks[ptr] = buf
	poolMu.Unlock()

	return ptr
}

// Free returns the allocated buffer to the slab pool for immediate zero-GC recycling.
//
//go:wasmexport acton_free
//export acton_free
func Free(ptr uint32, size uint32) {
	if ptr == 0 {
		return
	}

	poolMu.Lock()
	buf, exists := activeBlocks[ptr]
	if !exists {
		poolMu.Unlock()
		return
	}
	delete(activeBlocks, ptr)

	capSize := len(buf)
	if capSize == slabSmall && len(smallPool) < 128 {
		smallPool = append(smallPool, buf)
	} else if capSize == slabMedium && len(mediumPool) < 64 {
		mediumPool = append(mediumPool, buf)
	} else if capSize == slabLarge && len(largePool) < 32 {
		largePool = append(largePool, buf)
	}
	poolMu.Unlock()
}

// GetBuffer retrieves a direct slice reference to WASM linear memory.
func GetBuffer(ptr uint32, length uint32) []byte {
	if ptr == 0 || length == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)
}
