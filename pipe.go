package bufioprop

import (
	"errors"
	"io"
	"runtime"
	"sync/atomic"
)

const maxSpin = 16 // Spin lock prevent going down to channel syncs

// ErrClosedPipe is the error used for read or write operations on a closed pipe.
var ErrClosedPipe = errors.New("bufio: read/write on closed pipe")

// A pipe is the shared pipe structure underlying PipeReader and PipeWriter.
type pipe struct {
	buffer []byte // Internal buffer to pass the data through
	size   int32  // Total size of the buffer (same as buffer arg, just cast)
	free   int32  // Currently available space in the buffer

	inPos  int32 // Position in the buffer where input should be written
	outPos int32 // Position in the buffer from where output should be read

	inWake  chan struct{} // Signaler for the reader, if it's asleep
	outWake chan struct{} // Signaler for the writer, if it's asleep

	inQuit  chan struct{} // Quit channel when the reader terminates
	outQuit chan struct{} // Quit channel when the writer terminates

	inErr  error // If reader closed, error to give writes
	outErr error // If writer closed, error to give reads
}

// Pipe creates an asynchronous in-memory pipe.
//
// It can be used to connect code expecting an io.Reader with code expecting
// an io.Writer.
//
// Reads on one end are matched with writes on the other, but not directly
// between the two; rather data passes through an internal buffer.
//
// It is safe to call Read and Write in parallel with each other or with
// Close. Close will complete once pending I/O is done. Parallel calls to
// Read, and parallel calls to Write, are not safe!
func Pipe(buffer int) (*PipeReader, *PipeWriter) {
	p := &pipe{
		buffer: make([]byte, buffer),
		size:   int32(buffer),
		free:   int32(buffer),

		inWake:  make(chan struct{}, 1),
		outWake: make(chan struct{}, 1),

		inQuit:  make(chan struct{}),
		outQuit: make(chan struct{}),
	}
	return &PipeReader{p}, &PipeWriter{p}
}

func (p *pipe) read(b []byte) (written int, failure error) {
	// Short circuit if the output was already closed
	select {
	case <-p.outQuit:
		return 0, ErrClosedPipe
	default:
	}

	for {
		safeFree := atomic.LoadInt32(&p.free)

		// If there's no data available, sleep
		for i := 0; safeFree == p.size && i < maxSpin; i++ {
			runtime.Gosched()
			safeFree = atomic.LoadInt32(&p.free)
		}
		if safeFree == p.size {
			select {
			case <-p.outWake: // wake signal from reader
				continue

			case <-p.inQuit: // reader done, return
				// Check for buffer write/reader quit and above check race
				safeFree = atomic.LoadInt32(&p.free)
				if safeFree != p.size {
					continue
				}
				failure = p.inErr
				return

			case <-p.outQuit: // writer closed prematurely
				failure = ErrClosedPipe
				return
			}
		}
		// Write a batch of data
		limit := p.outPos + p.size - safeFree
		if limit > p.size {
			limit = p.size
		}
		if limit > p.outPos+int32(len(b)) {
			limit = p.outPos + int32(len(b))
		}
		copy(b, p.buffer[p.outPos:limit])
		nw := limit - p.outPos
		written += int(nw)

		// Update the write pointer and space availability
		p.outPos += int32(nw)
		if p.outPos >= p.size {
			p.outPos -= p.size
		}
		atomic.AddInt32(&p.free, int32(nw))

		// Signal the reader if it's asleep
		select {
		case p.inWake <- struct{}{}:
		default:
		}
		return
	}
}

func (p *pipe) readFrom(r io.Reader) (read int64, failure error) {
	for {
		safeFree := atomic.LoadInt32(&p.free)

		// If the buffer is full, wait
		for i := 0; safeFree == 0 && i < maxSpin; i++ {
			runtime.Gosched()
			safeFree = atomic.LoadInt32(&p.free)
		}
		if safeFree == 0 {
			select {
			case <-p.inWake: // wake signal from writer, retry
				continue

			case <-p.outQuit: // writer dead, return
				failure = ErrClosedPipe
				return
			}
		}
		// Try to fill the buffer either till the reader position, or the end
		limit := p.inPos + safeFree
		if limit > p.size {
			limit = p.size
		}
		nr, err := r.Read(p.buffer[p.inPos:limit])
		read += int64(nr)

		// Update the write pointer and space availability
		p.inPos += int32(nr)
		if p.inPos >= p.size {
			p.inPos -= p.size
		}
		atomic.AddInt32(&p.free, -int32(nr))

		// Handle any reader errors
		if err == io.EOF {
			return
		}
		if err != nil {
			failure = err
			return
		}
		// Signal the writer if it's asleep
		select {
		case p.outWake <- struct{}{}:
		default:
		}
	}
}

func (p *pipe) write(b []byte) (read int, failure error) {
	// Short circuit if the input was already closed
	select {
	case <-p.inQuit:
		return 0, ErrClosedPipe
	default:
	}

	for len(b) > 0 {
		safeFree := atomic.LoadInt32(&p.free)

		// If the buffer is full, wait
		for i := 0; safeFree == 0 && i < maxSpin; i++ {
			runtime.Gosched()
			safeFree = atomic.LoadInt32(&p.free)
		}
		if safeFree == 0 {
			select {
			case <-p.inWake: // wake signal from writer, retry
				continue

			case <-p.outQuit: // writer dead, return
				failure = ErrClosedPipe
				return

			case <-p.inQuit: // reader closed prematurely
				failure = ErrClosedPipe
				return
			}
		}
		// Try to fill the buffer either till the reader position, or the end
		limit := p.inPos + safeFree
		if limit > p.size {
			limit = p.size
		}
		if limit > p.inPos+int32(len(b)) {
			limit = p.inPos + int32(len(b))
		}
		copy(p.buffer[p.inPos:limit], b[:limit-p.inPos])
		nr := limit - p.inPos
		b = b[nr:]
		read += int(nr)

		// Update the write pointer and space availability
		p.inPos += int32(nr)
		if p.inPos >= p.size {
			p.inPos -= p.size
		}
		atomic.AddInt32(&p.free, -int32(nr))

		// Signal the writer if it's asleep
		select {
		case p.outWake <- struct{}{}:
		default:
		}
	}
	return
}

func (p *pipe) writeTo(w io.Writer) (written int64, failure error) {
	for {
		safeFree := atomic.LoadInt32(&p.free)

		// If there's no data available, sleep
		for i := 0; safeFree == p.size && i < maxSpin; i++ {
			runtime.Gosched()
			safeFree = atomic.LoadInt32(&p.free)
		}
		if safeFree == p.size {
			select {
			case <-p.outWake: // wake signal from reader
				continue

			case <-p.inQuit: // reader done, return
				// Check for buffer write/reader quit and above check race
				safeFree = atomic.LoadInt32(&p.free)
				if safeFree != p.size {
					continue
				}
				return
			}
		}
		// Write a batch of data
		limit := p.outPos + p.size - safeFree
		if limit > p.size {
			limit = p.size
		}
		nw, err := w.Write(p.buffer[p.outPos:limit])
		written += int64(nw)

		// Update the counters and check for errors
		if err != nil {
			failure = err
			return
		}
		if int32(nw) != limit-p.outPos {
			err = io.ErrShortWrite
			return
		}
		// Update the write pointer and space availability
		p.outPos += int32(nw)
		if p.outPos >= p.size {
			p.outPos -= p.size
		}
		atomic.AddInt32(&p.free, int32(nw))

		// Signal the reader if it's asleep
		select {
		case p.inWake <- struct{}{}:
		default:
		}
	}
}

func (p *pipe) rclose(err error) {
	p.outErr = err
	close(p.outQuit)
}

func (p *pipe) wclose(err error) {
	if err == nil {
		err = io.EOF
	}
	p.inErr = err
	close(p.inQuit)
}

// A PipeReader is the read half of a pipe.
type PipeReader struct {
	p *pipe
}

// Read reads data from the pipe. It returns io.EOF when the write side of the
// pipe has been closed and all the data has been read.
func (r *PipeReader) Read(data []byte) (n int, err error) {
	return r.p.read(data)
}

// WriteTo implements io.WriterTo by reading data from the pipe until EOF and
// writing it to w.
func (r *PipeReader) WriteTo(w io.Writer) (int64, error) {
	return r.p.writeTo(w)
}

// Close closes the reader; subsequent writes to the write half of the pipe will
// return the error ErrClosedPipe.
func (r *PipeReader) Close() error {
	return r.CloseWithError(nil)
}

// CloseWithError closes the reader; subsequent writes to the write half of the
// pipe will return the error err.
func (r *PipeReader) CloseWithError(err error) error {
	r.p.rclose(err)
	return nil
}

// A PipeWriter is the write half of a pipe.
type PipeWriter struct {
	p *pipe
}

// Write writes data to the pipe. It will block until all the data is written or
// the read half is closed.
func (w *PipeWriter) Write(data []byte) (n int, err error) {
	return w.p.write(data)
}

// ReadFrom implements io.ReaderFrom by reading all the data from r and writing
// it to the pipe.
func (w *PipeWriter) ReadFrom(r io.Reader) (int64, error) {
	return w.p.readFrom(r)
}

// Close closes the writer; subsequent reads from the read half of the pipe will
// return no bytes and EOF.
func (w *PipeWriter) Close() error {
	return w.CloseWithError(nil)
}

// CloseWithError closes the writer; subsequent reads from the read half of the
// pipe will return no bytes and the error err.
func (w *PipeWriter) CloseWithError(err error) error {
	w.p.wclose(err)
	return nil
}
