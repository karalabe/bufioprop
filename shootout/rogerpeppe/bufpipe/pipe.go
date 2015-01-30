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
type Reader struct {
	p *pipe
}

// Read reads data from the pipe. It returns io.EOF when
// the write side of the pipe has been closed and all
// the data has been read.
func (r *Reader) Read(data []byte) (int, error) {
	return r.p.read(data)
}

// Close closes the pipe. Subsequent writes to the write end
// of the pipe will return an io.ErrClosedPipe error.
func (r *Reader) Close() error {
	return r.p.closeReader()
}

// Writer holds the write half of a buffered pipe.
type Writer struct {
	p *pipe
}

// Write writes data to the pipe. It will block until all
// the data is written or the read half is closed.
func (w *Writer) Write(data []byte) (int, error) {
	return w.p.write(data)
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
	p.wClosed = true
	p.rwait.Broadcast()
	p.wwait.Broadcast()
	p.mu.Unlock()
	return nil
}

func (p *pipe) closeReader() error {
	p.mu.Lock()
	p.rClosed = true
	p.rwait.Broadcast()
	p.wwait.Broadcast()
	p.mu.Unlock()
	return nil
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
		p.rwait.Broadcast()
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
			p.wwait.Broadcast()
			return n, nil
		}
		if p.wClosed {
			return 0, io.EOF
		}
		p.rwait.Wait()
	}
}

// copyFrom copies as many bytes as possible from data into the ring
// buffer and returns the number of bytes copied.
func (p *pipe) copyFrom(data []byte) int {
	b0, b1 := p.bufs(p.p1, p.p0+int64(len(p.buf)))
	n := copy(b0, data)
	data = data[n:]
	n += copy(b1, data)
	p.p1 += int64(n)
	return n
}

// copyInto copies as many bytes as possible from the ring buffer into
// data and returns the number of bytes copied.
func (p *pipe) copyInto(data []byte) int {
	b0, b1 := p.bufs(p.p0, p.p1)
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
