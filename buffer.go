package ent

// Buffer is an extension to the byte array with functions for efficiently growing it,
// useful for constructing variably-sized byte arrays.
type Buffer []byte

// minimum size a nil buffer is automatically initialized to
const bufferMinAutoInitSize = 64

// NewBuffer creates a buffer with some preallocated space.
// Note that you do not need to use this function. Simply using a default-initialized Buffer
// works just as well as it will be allocated on first grow() call.
func NewBuffer(initcap int) Buffer {
	return Buffer(make([]byte, 0, initcap))
}

// Reset truncates the buffer's length to zero, allowing it to be reused.
func (b *Buffer) Reset() {
	*b = (*b)[:0]
}

// Write appends data to the buffer, returning the offset to the start of the appended data
func (b *Buffer) Write(data []byte) int {
	i := b.Grow(len(data))
	e := i + copy((*b)[i:], data)
	*b = (*b)[:e]
	return i
}

func (b *Buffer) WriteString(s string) {
	b.Write([]byte(s))
}

func (b *Buffer) WriteByte(v byte) {
	i := b.Grow(1)
	(*b)[i] = v
	*b = (*b)[:i+1]
}

func (b *Buffer) Reserve(n int) {
	if l := len(*b); n > cap(*b)-l {
		b.grow(n)
		*b = (*b)[:l]
	}
}

// Grow returns the index where bytes should be written and whether it succeeded.
func (b *Buffer) Grow(n int) int {
	l := len(*b)
	if n > cap(*b)-l {
		return b.grow(n)
	}
	*b = (*b)[:l+n]
	return l
}

// func (b *Buffer) growCap(n int) {
// 	l := len(*b)
// 	if cap(*buf)-l < n {
// 		b.grow(n)
// 		*b = (*b)[:l]
// 	}
// }

// grow the buffer to guarantee space for n more bytes.
// It returns the index where bytes should be written.
func (b *Buffer) grow(n int) int {
	// adapted from go's src/bytes/buffer.go
	m := len(*b)
	// // Try to grow by means of a reslice.
	// if i, ok := b.tryGrowByReslice(n); ok {
	// 	return i
	// }
	if *b == nil && n <= bufferMinAutoInitSize {
		// minimum initial allocation
		*b = make([]byte, n, bufferMinAutoInitSize)
		return 0
	}
	// Not enough space anywhere, we need to allocate.
	c := cap(*b)
	buf := make([]byte, 2*c+n)
	copy(buf, *b)
	*b = buf[:m+n]
	return m
}

func (b Buffer) Bytes() []byte {
	return []byte(b)
}

// DenseBytes returns the receiver if the density (cap divided by len) is less than
// densityThreshold. Otherwise a perfectly dense copy is returned.
//
// This is useful if you plan to keep a lot of buffers unmodified for a long period of time,
// where memory consumption might be a concern.
//
// To always make a copy, provide a densityThreshold of 1.0 or lower.
//
func (b Buffer) DenseBytes(densityThreshold float64) []byte {
	if float64(cap(b))/float64(len(b)) > densityThreshold {
		buf := make([]byte, len(b))
		copy(buf, b)
		return buf
	}
	return b
}
