// Package perf provides performance optimizations for Sol Trade SDK
package perf

import (
	"unsafe"
)

// Export common types and functions

// CacheLineSize is the typical CPU cache line size
const CacheLineSize = 64

// PadToCacheLine pads a struct size to cache line boundary
func PadToCacheLine(size int) int {
	return (size + CacheLineSize - 1) &^ (CacheLineSize - 1)
}

// CacheLinePad provides padding to prevent false sharing
type CacheLinePad [CacheLineSize]byte

// FalseSharingPrevention embed this to prevent false sharing
type FalseSharingPrevention struct {
	_ CacheLinePad
}

// NoCopy is a struct that should not be copied
type NoCopy struct{}

// Lock is a no-op for NoCopy
func (*NoCopy) Lock() {}

// Unlock is a no-op for NoCopy
func (*NoCopy) Unlock() {}

// Pointer arithmetic helpers

// Add adds offset to pointer
func Add(p unsafe.Pointer, offset uintptr) unsafe.Pointer {
	return unsafe.Pointer(uintptr(p) + offset)
}

// Align aligns pointer to boundary
func Align(p unsafe.Pointer, boundary uintptr) unsafe.Pointer {
	addr := uintptr(p)
	aligned := (addr + boundary - 1) &^ (boundary - 1)
	return unsafe.Pointer(aligned)
}

// IsAligned checks if pointer is aligned
func IsAligned(p unsafe.Pointer, boundary uintptr) bool {
	return uintptr(p)%boundary == 0
}
