package perf

import (
	"sync"
	"sync/atomic"
)

// ZeroCopyBuffer provides zero-copy buffer operations
type ZeroCopyBuffer struct {
	buf  []byte
	pos  int
	size int
}

// NewZeroCopyBuffer creates a zero-copy buffer
func NewZeroCopyBuffer(size int) *ZeroCopyBuffer {
	return &ZeroCopyBuffer{
		buf:  make([]byte, size),
		size: size,
	}
}

// NewZeroCopyBufferFromSlice creates buffer from existing slice
func NewZeroCopyBufferFromSlice(buf []byte) *ZeroCopyBuffer {
	return &ZeroCopyBuffer{
		buf:  buf,
		size: len(buf),
	}
}

// Write writes data without copying if possible
func (b *ZeroCopyBuffer) Write(p []byte) (int, error) {
	if b.pos+len(p) > b.size {
		return 0, nil // Would overflow
	}
	copy(b.buf[b.pos:], p)
	b.pos += len(p)
	return len(p), nil
}

// WriteByte writes a single byte
func (b *ZeroCopyBuffer) WriteByte(c byte) error {
	if b.pos >= b.size {
		return nil
	}
	b.buf[b.pos] = c
	b.pos++
	return nil
}

// Read reads data into p
func (b *ZeroCopyBuffer) Read(p []byte) (int, error) {
	n := copy(p, b.buf[:b.pos])
	return n, nil
}

// Bytes returns the buffer contents without copy
func (b *ZeroCopyBuffer) Bytes() []byte {
	return b.buf[:b.pos]
}

// Slice returns a slice of the buffer (zero-copy view)
func (b *ZeroCopyBuffer) Slice(start, end int) []byte {
	if start < 0 || end > b.pos || start > end {
		return nil
	}
	return b.buf[start:end]
}

// Reset resets the buffer for reuse
func (b *ZeroCopyBuffer) Reset() {
	b.pos = 0
}

// Len returns current length
func (b *ZeroCopyBuffer) Len() int {
	return b.pos
}

// Cap returns capacity
func (b *ZeroCopyBuffer) Cap() int {
	return b.size
}

// BufferPool provides a pool of reusable buffers
type BufferPool struct {
	pool   chan *ZeroCopyBuffer
	size   int
	maxCap int
}

// NewBufferPool creates a buffer pool
func NewBufferPool(poolSize, bufferSize int) *BufferPool {
	return &BufferPool{
		pool:   make(chan *ZeroCopyBuffer, poolSize),
		size:   bufferSize,
		maxCap: poolSize,
	}
}

// Get gets a buffer from pool
func (p *BufferPool) Get() *ZeroCopyBuffer {
	select {
	case buf := <-p.pool:
		buf.Reset()
		return buf
	default:
		return NewZeroCopyBuffer(p.size)
	}
}

// Put returns a buffer to pool
func (p *BufferPool) Put(buf *ZeroCopyBuffer) {
	if buf == nil {
		return
	}
	buf.Reset()
	select {
	case p.pool <- buf:
	default:
		// Pool full, let GC collect
	}
}

// ZeroCopySerializer provides zero-copy serialization
type ZeroCopySerializer struct {
	buffer *ZeroCopyBuffer
	pool   *BufferPool
}

// NewZeroCopySerializer creates a serializer
func NewZeroCopySerializer(pool *BufferPool) *ZeroCopySerializer {
	return &ZeroCopySerializer{
		buffer: pool.Get(),
		pool:   pool,
	}
}

// Release releases the serializer back to pool
func (s *ZeroCopySerializer) Release() {
	if s.pool != nil && s.buffer != nil {
		s.pool.Put(s.buffer)
		s.buffer = nil
	}
}

// WriteU8 writes uint8
func (s *ZeroCopySerializer) WriteU8(v uint8) {
	s.buffer.WriteByte(v)
}

// WriteU16 writes uint16 (little endian)
func (s *ZeroCopySerializer) WriteU16(v uint16) {
	s.buffer.WriteByte(byte(v))
	s.buffer.WriteByte(byte(v >> 8))
}

// WriteU32 writes uint32 (little endian)
func (s *ZeroCopySerializer) WriteU32(v uint32) {
	s.buffer.WriteByte(byte(v))
	s.buffer.WriteByte(byte(v >> 8))
	s.buffer.WriteByte(byte(v >> 16))
	s.buffer.WriteByte(byte(v >> 24))
}

// WriteU64 writes uint64 (little endian)
func (s *ZeroCopySerializer) WriteU64(v uint64) {
	for i := 0; i < 8; i++ {
		s.buffer.WriteByte(byte(v >> (i * 8)))
	}
}

// WriteBytes writes byte slice
func (s *ZeroCopySerializer) WriteBytes(p []byte) {
	s.buffer.Write(p)
}

// WriteCompactU16 writes compact u16
func (s *ZeroCopySerializer) WriteCompactU16(v uint16) {
	if v < 0x80 {
		s.WriteU8(uint8(v))
	} else if v < 0x4000 {
		s.WriteU8(uint8(v&0x7F | 0x80))
		s.WriteU8(uint8(v >> 7))
	} else {
		s.WriteU8(uint8(v&0x7F | 0x80))
		s.WriteU8(uint8((v>>7)&0x7F | 0x80))
		s.WriteU8(uint8(v >> 14))
	}
}

// Bytes returns serialized bytes
func (s *ZeroCopySerializer) Bytes() []byte {
	return s.buffer.Bytes()
}

// SliceAllocator provides efficient slice allocation
type SliceAllocator struct {
	pool sync.Pool
	size int
}

// NewSliceAllocator creates a slice allocator
func NewSliceAllocator(size int) *SliceAllocator {
	return &SliceAllocator{
		pool: sync.Pool{
			New: func() interface{} {
				b := make([]byte, size)
				return &b
			},
		},
		size: size,
	}
}

// Alloc allocates a slice
func (a *SliceAllocator) Alloc() []byte {
	bp := a.pool.Get().(*[]byte)
	return (*bp)[:a.size]
}

// Free returns a slice to pool
func (a *SliceAllocator) Free(p []byte) {
	if cap(p) >= a.size {
		a.pool.Put(&p)
	}
}

// AtomicBuffer provides atomic buffer operations
type AtomicBuffer struct {
	buf  []byte
	size int32
	cap  int
}

// NewAtomicBuffer creates an atomic buffer
func NewAtomicBuffer(capacity int) *AtomicBuffer {
	return &AtomicBuffer{
		buf: make([]byte, capacity),
		cap: capacity,
	}
}

// Write writes data atomically
func (b *AtomicBuffer) Write(p []byte) bool {
	currentSize := atomic.LoadInt32(&b.size)
	newSize := currentSize + int32(len(p))
	if int(newSize) > b.cap {
		return false
	}

	if atomic.CompareAndSwapInt32(&b.size, currentSize, newSize) {
		copy(b.buf[currentSize:newSize], p)
		return true
	}
	return false
}

// Read reads all data
func (b *AtomicBuffer) Read() []byte {
	size := atomic.LoadInt32(&b.size)
	return b.buf[:size]
}

// Reset resets buffer
func (b *AtomicBuffer) Reset() {
	atomic.StoreInt32(&b.size, 0)
}
