package bufioprop

import (
	"errors"
	"io"
	"runtime"
	"sync"
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

	inQuit      chan struct{} // Quit channel when the reader terminates
	outQuit     chan struct{} // Quit channel when the writer terminates
	outQuitLock sync.Mutex    // Lock to prevent multiple quit channel closes

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
func (r *PipeReader) WriteTo(w io.Writer) (written int64, err error) {
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
	r.p.outputClose(err)
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
func (w *PipeWriter) ReadFrom(r io.Reader) (read int64, err error) {
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
	w.p.inputClose(err)
	return nil
}

// InputWait blocks until some space frees up in the internal buffer.
func (p *pipe) inputWait() (int32, error) {
	for {
		safeFree := atomic.LoadInt32(&p.free)

		// If the buffer is full, spin lock to give it another chance
		for i := 0; safeFree == 0 && i < maxSpin; i++ {
			runtime.Gosched()
			safeFree = atomic.LoadInt32(&p.free)
		}
		// If still full, go down into deep sleep
		if safeFree == 0 {
			select {
			case <-p.inWake: // wake signal from output, retry
				continue

			case <-p.outQuit: // output dead, return
				return safeFree, ErrClosedPipe

			case <-p.inQuit: // input closed prematurely
				return safeFree, ErrClosedPipe
			}
		}
		return safeFree, nil
	}
}

// OutputWait blocks until some data becomes available in the internal buffer.
func (p *pipe) outputWait() (int32, error) {
	for {
		safeFree := atomic.LoadInt32(&p.free)

		// If there's no data available, spin lock to give it another chance
		for i := 0; safeFree == p.size && i < maxSpin; i++ {
			runtime.Gosched()
			safeFree = atomic.LoadInt32(&p.free)
		}
		// If still no data, go down into deep sleep
		if safeFree == p.size {
			select {
			case <-p.outWake: // wake signal from input, retry
				continue

			case <-p.inQuit: // input done, return
				safeFree = atomic.LoadInt32(&p.free)
				if safeFree != p.size {
					return safeFree, nil
				}
				p.outputClose(nil)
				return safeFree, p.inErr

			case <-p.outQuit: // output closed prematurely
				return safeFree, ErrClosedPipe
			}
		}
		return safeFree, nil
	}
}

// InputAdvance updates the input index, buffer free space counter and signals
// the output writer (if any) that space is available.
func (p *pipe) inputAdvance(count int) {
	p.inPos += int32(count)
	if p.inPos >= p.size {
		p.inPos -= p.size
	}
	atomic.AddInt32(&p.free, -int32(count))

	select {
	case p.outWake <- struct{}{}:
	default:
	}
}

// OutputAdvance updates the output index, buffer free space counter and signals
// the input writer (if any) that space is available.
func (p *pipe) outputAdvance(count int) {
	p.outPos += int32(count)
	if p.outPos >= p.size {
		p.outPos -= p.size
	}
	atomic.AddInt32(&p.free, int32(count))

	select {
	case p.inWake <- struct{}{}:
	default:
	}
}

// Read fills a buffer with any available data, returning as soon as something's
// been read.
func (p *pipe) read(b []byte) (int, error) {
	// Short circuit if the output was already closed
	select {
	case <-p.outQuit:
		return 0, ErrClosedPipe
	default:
	}
	// Wait until some data becomes available
	safeFree, err := p.outputWait()
	if err != nil {
		return 0, err
	}
	// Retrieve as much as available
	limit := p.outPos + p.size - safeFree
	if limit > p.size {
		limit = p.size
	}
	if limit > p.outPos+int32(len(b)) {
		limit = p.outPos + int32(len(b))
	}
	written := copy(b, p.buffer[p.outPos:limit])

	// Update the pipe output state and return
	p.outputAdvance(written)
	return written, nil
}

// WriteTo keeps pushing data into the writer until the source is closed or fails.
func (p *pipe) writeTo(w io.Writer) (written int64, err error) {
	for {
		// Wait until some data becomes available
		safeFree, err := p.outputWait()
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return written, err
		}
		// Try and write it all
		limit := p.outPos + p.size - safeFree
		if limit > p.size {
			limit = p.size
		}
		nw, err := w.Write(p.buffer[p.outPos:limit])
		written += int64(nw)

		// Update the counters and check for errors
		if err != nil {
			return written, err
		}
		if int32(nw) != limit-p.outPos {
			return written, io.ErrShortWrite
		}
		// Update the pipe output state and return
		p.outputAdvance(nw)
	}
}

// Write pushes the contents of a slice into the internal data buffer.
func (p *pipe) write(b []byte) (read int, failure error) {
	// Short circuit if the input was already closed
	select {
	case <-p.inQuit:
		return 0, ErrClosedPipe
	default:
	}

	for len(b) > 0 {
		// Wait until some space frees up
		safeFree, err := p.inputWait()
		if err != nil {
			return read, err
		}
		// Try to fill the buffer either till the reader position, or the end
		limit := p.inPos + safeFree
		if limit > p.size {
			limit = p.size
		}
		if limit > p.inPos+int32(len(b)) {
			limit = p.inPos + int32(len(b))
		}
		nr := copy(p.buffer[p.inPos:limit], b[:limit-p.inPos])
		b = b[nr:]
		read += int(nr)

		// Update the pipe input state and continue
		p.inputAdvance(nr)
	}
	return
}

// ReadFrom keeps fetching data from the reader and placing it into the internal
// buffer as long as the stream is live.
func (p *pipe) readFrom(r io.Reader) (read int64, failure error) {
	for {
		// Wait until some space frees up
		safeFree, err := p.inputWait()
		if err != nil {
			return read, err
		}
		// Try to fill the buffer either till the reader position, or the end
		limit := p.inPos + safeFree
		if limit > p.size {
			limit = p.size
		}
		nr, err := r.Read(p.buffer[p.inPos:limit])
		read += int64(nr)

		// Update the pipe input state and handle any occurred errors
		p.inputAdvance(nr)
		if err == io.EOF {
			return read, nil
		}
		if err != nil {
			return read, err
		}
	}
}

// OutputClose terminates the writer endpoint, notifying further reads of the
// specified error.
func (p *pipe) outputClose(err error) {
	p.outQuitLock.Lock()
	defer p.outQuitLock.Unlock()

	p.outErr = err
	select {
	case <-p.outQuit:
		return
	default:
		close(p.outQuit)
	}
}

// InputClose terminates the reader endpoint, notifying any reads after the
// buffer is flushed of it. In case of a nil close, EOF is returned.
func (p *pipe) inputClose(err error) {
	if err == nil {
		err = io.EOF
	}
	p.inErr = err

	close(p.inQuit)
	if atomic.LoadInt32(&p.free) != p.size {
		<-p.outQuit
	}
}
