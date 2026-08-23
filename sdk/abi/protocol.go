package abi

// PackPtrLen packs a 32-bit pointer and 32-bit length into a single uint64.
// Format: (ptr << 32) | len
func PackPtrLen(ptr uint32, length uint32) uint64 {
	return (uint64(ptr) << 32) | uint64(length)
}

// UnpackPtrLen unpacks a uint64 into a 32-bit pointer and 32-bit length.
func UnpackPtrLen(packed uint64) (ptr uint32, length uint32) {
	ptr = uint32(packed >> 32)
	length = uint32(packed & 0xFFFFFFFF)
	return ptr, length
}

// BytesToPtr converts a Go byte slice to a pointer and length for WASM linear memory.
func BytesToPtr(b []byte) (uint32, uint32) {
	if len(b) == 0 {
		return 0, 0
	}
	size := uint32(len(b))
	ptr := Alloc(size)
	if ptr == 0 {
		return 0, 0
	}
	dest := GetBuffer(ptr, size)
	if dest != nil {
		copy(dest, b)
	}
	return ptr, size
}

// PtrToBytes reads a byte slice from a given pointer and length.
func PtrToBytes(ptr uint32, length uint32) []byte {
	if ptr == 0 || length == 0 {
		return nil
	}
	src := GetBuffer(ptr, length)
	if src == nil {
		return nil
	}
	buf := make([]byte, length)
	copy(buf, src)
	return buf
}

// StringToPtr converts a Go string to a pointer and length.
func StringToPtr(s string) (uint32, uint32) {
	return BytesToPtr([]byte(s))
}

// PtrToString converts a pointer and length into a Go string.
func PtrToString(ptr uint32, length uint32) string {
	b := PtrToBytes(ptr, length)
	return string(b)
}
