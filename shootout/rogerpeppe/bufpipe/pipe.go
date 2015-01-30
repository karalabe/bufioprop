package bufpipe

import (
	"io"
	"sync"
)

// pipe is the internal representation of a buffered pipe.
// It is referred to by both Reader and Writer.
type pipe struct {
	// mu guards all the fields in the struct.
	mu sync.Mutex

	// rwait is a condition that can become true when there
	// is some data written to the pipe.
	rwait sync.Cond

	// wwait is a condition that can become true when some
	// space has become available in the pipe.
	wwait sync.Cond

	// rClosed holds whether the reader side of the pipe has
	// been closed.
	rClosed bool

	// wClosed holds whether the writer side of the pipe has
	// been closed.
	wClosed bool

	// buf holds the ring buffer itself.
	buf []byte

	// [p0, p1) holds the range of bytes currently populating
	// the ring buffer. Counting starts from 0 (when the pipe
	// was created). We assume the numbers never wrap.
	p0, p1 int64
}

// Reader holds the read half of a buffered pipe.
// It is not safe to call methods concurrently on a Reader value.
type Reader struct {
	p *pipe
}

// Read reads data from the pipe. It returns io.EOF when
// the write side of the pipe has been closed and all
// the data has been read.
func (r *Reader) Read(data []byte) (int, error) {
	return r.p.read(data)
}

// WriteTo implements io.WriterTo by reading
// data from the pipe until EOF and writing it to w.
func (r *Reader) WriteTo(w io.Writer) (int64, error) {
	return r.p.writeTo(w)
}

// Close closes the pipe. Subsequent writes to the write end
// of the pipe will return an io.ErrClosedPipe error.
func (r *Reader) Close() error {
	return r.p.closeReader()
}

// Writer holds the write half of a buffered pipe.
// It is not safe to call methods concurrently on
// Writer value.
type Writer struct {
	p *pipe
}

// Write writes data to the pipe. It will block until all
// the data is written or the read half is closed.
func (w *Writer) Write(data []byte) (int, error) {
	return w.p.write(data)
}

// ReadFrom implements io.ReaderFrom by reading
// all the data from r and writing it to the pipe.
func (w *Writer) ReadFrom(r io.Reader) (int64, error) {
	return w.p.readFrom(r)
}

// Close closes the pipe.
func (w *Writer) Close() error {
	return w.p.closeWriter()
}

// New returns both halves of a new buffered pipe
// using the given buffer size. It is OK to use methods
// concurrently on the returned values.
func New(size int) (*Reader, *Writer) {
	p := &pipe{
		buf: make([]byte, size),
	}
	p.rwait.L = &p.mu
	p.wwait.L = &p.mu
	return &Reader{p}, &Writer{p}
}

func (p *pipe) closeWriter() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.wClosed = true
	p.rwait.Signal()
	p.wwait.Signal()
	return nil
}

func (p *pipe) closeReader() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rClosed = true
	p.rwait.Signal()
	p.wwait.Signal()
	return nil
}

func (p *pipe) readFrom(r io.Reader) (int64, error) {
	nw := int64(0)
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		if p.rClosed {
			return nw, io.ErrClosedPipe
		}
		buf, _ := p.writeBuf()
		if len(buf) == 0 {
			p.wwait.Wait()
			continue
		}
		for len(buf) > 0 {
			// Unlock the mutex and read directly into
			// the buffer. This is OK because the available
			// buffer space can only ever decrease (assuming
			// there are no other concurrent writers).
			p.mu.Unlock()
			n, err := r.Read(buf)
			p.mu.Lock()

			nw += int64(n)
			p.p1 += int64(n)
			p.rwait.Signal()
			if err != nil {
				if err == io.EOF {
					err = nil
				}
				return nw, err
			}
			buf = buf[n:]
		}
	}
}

func (p *pipe) write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	nw := 0
	for {
		if p.rClosed {
			return nw, io.ErrClosedPipe
		}
		n := p.copyFrom(data)
		p.rwait.Signal()
		data = data[n:]
		nw += n
		if len(data) == 0 {
			break
		}
		if p.isFull() {
			p.wwait.Wait()
		}
	}
	return nw, nil
}

func (p *pipe) isFull() bool {
	return int(p.p1-p.p0) >= len(p.buf)
}

func (p *pipe) isEmpty() bool {
	return p.p0 == p.p1
}

func (p *pipe) read(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		n := p.copyInto(data)
		data = data[n:]
		if n > 0 {
			p.wwait.Signal()
			return n, nil
		}
		if p.wClosed {
			return 0, io.EOF
		}
		p.rwait.Wait()
	}
}

func (p *pipe) writeTo(w io.Writer) (int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	nr := int64(0)
	for {
		buf, _ := p.readBuf()
		if len(buf) == 0 {
			if p.wClosed {
				return nr, nil
			}
			p.rwait.Wait()
			continue
		}
		// Unlock the mutex and write directly from
		// the buffer. This is OK because the available
		// buffered data can only ever increase (assuming
		// no other concurrent readers)
		p.mu.Unlock()
		n, err := w.Write(buf)
		p.mu.Lock()

		nr += int64(n)
		p.p0 += int64(n)
		if err != nil {
			return nr, err
		}
		p.wwait.Signal()
	}
}

func (p *pipe) writeBuf() ([]byte, []byte) {
	return p.bufs(p.p1, p.p0+int64(len(p.buf)))
}

func (p *pipe) readBuf() ([]byte, []byte) {
	return p.bufs(p.p0, p.p1)
}

// copyFrom copies as many bytes as possible from data into the ring
// buffer and returns the number of bytes copied.
func (p *pipe) copyFrom(data []byte) int {
	b0, b1 := p.writeBuf()
	n := copy(b0, data)
	data = data[n:]
	n += copy(b1, data)
	p.p1 += int64(n)
	return n
}

// copyInto copies as many bytes as possible from the ring buffer into
// data and returns the number of bytes copied.
func (p *pipe) copyInto(data []byte) int {
	b0, b1 := p.readBuf()
	n := copy(data, b0)
	data = data[n:]
	n += copy(data, b1)
	p.p0 += int64(n)
	return n
}

// bufs returns the slices of p.buf corresponding to the range from
// offset p0 to p1, where p1-p0 must be >=0 and <= len(p.buf). Because
// the buffer wraps, there can be at most two such slices.
func (p *pipe) bufs(p0, p1 int64) ([]byte, []byte) {
	buf := p.buf
	start := int(p0 % int64(len(buf)))
	n := int(p1 - p0)
	if start+n <= len(buf) {
		return buf[start : start+n], nil
	}
	b0 := buf[start:]
	n -= len(buf) - start
	b1 := buf[0:n]
	return b0, b1
}
