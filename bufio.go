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
	size := int32(buffer) // Total size of the buffer (same as buffer arg, just cast)
	free := int32(buffer) // Currently available space in the buffer

	inPos := int32(0)  // Position in the buffer where input should be written
	outPos := int32(0) // Position in the buffer from where output should be read

	inWake := make(chan struct{}, 1)  // signaler for the reader, if it's asleep
	outWake := make(chan struct{}, 1) // signaler for the writer, if it's asleep

	inQuit := make(chan struct{})  // quit channel when the reader terminates
	outQuit := make(chan struct{}) // quit channel when the writer terminates

	// Start a reader goroutine that pushes data into the buffer
	go func() {
		defer close(inQuit)

		for {
			safeFree := atomic.LoadInt32(&free)

			// If the buffer is full, wait
			if safeFree == 0 {
				select {
				case <-inWake: // wake signal from writer, retry
					continue

				case <-outQuit: // writer dead, return
					return
				}
			}
			// Try to fill the buffer either till the reader position, or the end
			nr := 0
			var er error

			if inPos+safeFree <= size { // reader in front of writer
				nr, er = src.Read(buf[inPos : inPos+safeFree])
			} else {
				nr, er = src.Read(buf[inPos:])
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
			inPos += int32(nr)
			if inPos >= size {
				inPos -= size
			}
			atomic.AddInt32(&free, -int32(nr))

			// Signal the writer if it's asleep
			select {
			case outWake <- struct{}{}:
			default:
			}
		}
	}()

	// Start a writer goroutine that retrieves data from the buffer
	go func() {
		defer close(outQuit)

		for {
			safeFree := atomic.LoadInt32(&free)

			// If there's no data available, sleep
			if safeFree == size {
				select {
				case <-outWake: // wake signal from reader
					continue

				case <-inQuit: // reader done, return
					// Check for buffer write/reader quit and above check race
					safeFree = atomic.LoadInt32(&free)
					if safeFree != size {
						continue
					}
					return
				}
			}
			// Write a batch of data
			nw, expect := 0, int32(0)
			var we error

			if outPos-safeFree <= 0 { // writer is in front of reader
				expect = size - safeFree
				nw, we = dst.Write(buf[outPos : outPos+expect])
			} else {
				expect = size - outPos
				nw, we = dst.Write(buf[outPos:])
			}
			written += int64(nw)

			// Update the counters and check for errors
			if we != nil {
				err = we
				return
			}
			if int32(nw) != expect {
				err = io.ErrShortWrite
				return
			}
			// Update the write pointer and space availability
			outPos += int32(nw)
			if outPos >= size {
				outPos -= size
			}
			atomic.AddInt32(&free, int32(nw))

			// Signal the reader if it's asleep
			select {
			case inWake <- struct{}{}:
			default:
			}
		}
	}()
	// Wait until both finish and return
	<-outQuit
	<-inQuit
	return
}
