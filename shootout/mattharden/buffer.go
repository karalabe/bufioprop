package mattharden

import (
	"bytes"
	"io"
	"sync"
)

// A BufferedPipe is a variable-sized buffer of bytes with Read and Write methods.
// Unlike bytes.Buffer, Write and Read can be called concurrently.
type BufferedPipe struct {
	mu     sync.Mutex
	c      sync.Cond
	closed bool
	buf    bytes.Buffer
}

func (b *BufferedPipe) Init(size int) {
	b.c.L = &b.mu
	b.buf.Grow(size)
}

// Read returns the next len(p) bytes from the buffer or until the buffer is drained.
// If the buffer is empty, Read waits until there are bytes available. If the buffer
// is empty and closed, Read returns err = io.EOF. Otherwise err is always nil.
func (b *BufferedPipe) Read(p []byte) (n int, err error) {
	b.mu.Lock()
	for !b.closed && b.buf.Len() == 0 {
		b.c.Wait()
	}
	n, err = b.buf.Read(p)
	b.mu.Unlock()
	return n, err
}

// Write writes data to the buffer, allocating more buffer space if necessary.
// If the buffer becomes too large, Write will panic with bytes.ErrTooLarge.
// Calling Write on a closed buffer returns err = io.ErrClosedPipe.
// Otherwise Write always returns err = nil.
func (b *BufferedPipe) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	if !b.closed {
		bcast := b.buf.Len() == 0
		n, err = b.buf.Write(p)
		if bcast {
			b.c.Broadcast()
		}
	} else {
		err = io.ErrClosedPipe
	}
	b.mu.Unlock()
	return n, err
}

// Close closes the Buffer. Close is idempotent; it can be called more than
// once on the same buffer with the same result as calling it once.
// Close always returns nil.
func (b *BufferedPipe) Close() error {
	b.mu.Lock()
	if !b.closed {
		b.closed = true
		if b.buf.Len() == 0 {
			b.c.Broadcast()
		}
	}
	b.mu.Unlock()
	return nil
}
