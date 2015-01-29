// Package bufioprop contains extension functions to the bufio package.
package ncw

import (
	"io"
	"sync"
)

// Copy copies from src to dst until either EOF is reached on src or an error
// occurs. It returns the number of bytes copied and the first error encountered
// while copying, if any.
//
// A successful Copy returns err == nil, not err == EOF. Because Copy is defined
// to read from src until EOF, it does not treat an EOF from Read as an error to
// be reported.
//
// Internally, one goroutine is reading the src, moving the data into an internal
// buffer, and another moving from the buffer to the writer. This permits both
// endpoints to run simultaneously, without one blocking the other.
func Copy(dst io.Writer, src io.Reader, buffer int) (written int64, err error) {
	// Pool of buffers for upload
	bufPool := sync.Pool{
		New: func() interface{} {
			return make([]byte, buffer)
		},
	}

	const inFlight = 1
	type chunk struct {
		buf []byte
		n   int
	}
	chunks := make(chan chunk, inFlight)
	errs := make(chan error, inFlight)

	// Read chunks from the src
	finished := make(chan struct{})
	go func() {
	loop:
		for {
			buf := bufPool.Get().([]byte)
			n, err := io.ReadFull(src, buf)
			select {
			case chunks <- chunk{
				buf: buf,
				n:   n,
			}:
			case <-finished:
				break loop
			}
			switch err {
			case nil:
			case io.EOF, io.ErrUnexpectedEOF:
				break loop
			default:
				errs <- err
				break loop
			}
		}
		close(chunks)
	}()

	// Write chunks to the dst
	go func() {
		for chunk := range chunks {
			n, err := dst.Write(chunk.buf[:chunk.n])
			written += int64(n)
			bufPool.Put(chunk.buf)
			if err != nil {
				errs <- err
				break
			}
		}
		close(errs)
	}()

	// Collect errors and return them
	for err = range errs {
		close(finished)
	}
	return
}
