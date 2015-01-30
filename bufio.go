// Package bufioprop contains extension functions to the bufio package.
package bufioprop

import (
	"io"
	"sync/atomic"
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
	buf := make([]byte, buffer)
	bs, ba, rp, wp := int32(buffer), int32(buffer), int32(0), int32(0)

	rs := make(chan struct{}, 1) // signaler for the reader, if it's asleep
	ws := make(chan struct{}, 1) // signaler for the writer, if it's asleep

	rq := make(chan struct{}) // quit channel when the reader terminates
	wq := make(chan struct{}) // quit channel when the writer terminates

	// Start a reader goroutine that pushes data into the buffer
	go func() {
		defer close(rq)

		for {
			bac := atomic.LoadInt32(&ba)

			// If the buffer is full, wait
			if bac == 0 {
				select {
				case <-rs: // wake signal from writer, retry
					continue

				case <-wq: // writer dead, return
					return
				}
			}
			// Try to fill the buffer either till the reader position, or the end
			nr := 0
			var er error

			if wp+bac <= bs { // reader in front of writer
				nr, er = src.Read(buf[wp : wp+bac])
			} else {
				nr, er = src.Read(buf[wp:])
			}
			// Handle any reader errors
			if er == io.EOF {
				break
			}
			if er != nil {
				err = er
				return
			}
			// Update the write pointer and space availability
			wp += int32(nr)
			if wp >= bs {
				wp -= bs
			}
			atomic.AddInt32(&ba, -int32(nr))

			// Signal the writer if it's asleep
			select {
			case ws <- struct{}{}:
			default:
			}
		}
	}()

	// Start a writer goroutine that retrieves data from the buffer
	go func() {
		defer close(wq)

		for {
			bac := atomic.LoadInt32(&ba)

			// If there's no data available, sleep
			if bac == bs {
				select {
				case <-ws: // wake signal from reader
					continue

				case <-rq: // reader done, return
					// Check for buffer write/reader quit and above check race
					bac = atomic.LoadInt32(&ba)
					if bac != bs {
						continue
					}
					return
				}
			}
			// Write a batch of data
			nw, nc := 0, int32(0)
			var we error

			if rp-bac <= 0 { // writer is in front of reader
				nc = bs - bac
				nw, we = dst.Write(buf[rp : rp+nc])
			} else {
				nc = bs - rp
				nw, we = dst.Write(buf[rp:])
			}
			written += int64(nw)
			if we != nil {
				err = we
				return
			}
			if nw != int(nc) {
				err = io.ErrShortWrite
				return
			}
			// Update the write pointer and space availability
			rp += int32(nw)
			if rp >= bs {
				rp -= bs
			}
			atomic.AddInt32(&ba, int32(nw))

			// Signal the reader if it's asleep
			select {
			case rs <- struct{}{}:
			default:
			}
		}
	}()

	// Wait until both finish and return
	<-wq
	<-rq
	return
}
