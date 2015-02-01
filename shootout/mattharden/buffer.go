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

// Read returns the next len(p) bytes from the buffer or until the buffer is drained.
// If the buffer is empty, Read waits until there are bytes available. If the buffer
// is empty and closed, Read returns err = io.EOF. Otherwise err is always nil.
func (b *BufferedPipe) Read(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.c.L == nil {
		b.c.L = &b.mu
	}
	for {
		if b.buf.Len() == 0 && b.closed {
			return 0, io.EOF
		}
		n, err = b.buf.Read(p)
		if err != io.EOF {
			return n, err
		}
		b.c.Wait()
	}
}

// Write writes data to the buffer, allocating more buffer space if necessary.
// If the buffer becomes too large, Write will panic with bytes.ErrTooLarge.
// Calling Write on a closed buffer returns err = io.ErrClosedPipe.
// Otherwise Write always returns err = nil.
func (b *BufferedPipe) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.c.L == nil {
		b.c.L = &b.mu
	}
	if b.closed {
		return 0, io.ErrClosedPipe
	}
	defer b.c.Broadcast()
	n, err = b.buf.Write(p)
	return n, err
}

// Close closes the Buffer. Close is idempotent; it can be called more than
// once on the same buffer with the same result as calling it once.
// Close always returns nil.
func (b *BufferedPipe) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	b.c.Broadcast()
	return nil
}
