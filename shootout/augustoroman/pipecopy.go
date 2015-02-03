package augustoroman

import (
	"fmt"
	"io"
	"runtime"
	"sync/atomic"
)

// Pipe returns a buffered pipe that uses the provided byte array as a data
// buffer. Providing a zero-length buffer will result in an inoperable pipe.
func Pipe(buf []byte) (Reader, Writer) {
	p := &pipe{
		Buf:         buf,
		dataReady:   make(chan token, 1),
		bufferReady: make(chan token, 1),
	}
	return Reader{p}, Writer{p}
}

func Copy(dst io.Writer, src io.Reader, bufferSize int) (int64, error) {
	return CopyWithBuffer(dst, src, make([]byte, bufferSize))
}

// Uses the provided buffer to copy src to dst.  Passing a zero-length buffer
// (e.g. nil) will perform an unbuffered copy.
func CopyWithBuffer(dst io.Writer, src io.Reader, buffer []byte) (int64, error) {
	if len(buffer) == 0 {
		return io.Copy(dst, src) // unbuffered copy
	}
	pr, pw := Pipe(buffer)
	errs := make(chan error, 1)
	go func() {
		_, err := io.Copy(pw, src)
		if err != nil {
			errs <- err
		}
		pw.close()
	}()
	n, err := io.Copy(dst, pr)
	errs <- err
	return n, <-errs // Return first error that came up.
}

type token struct{}

type pipe struct {
	Buf []byte

	// atomic'd
	bytesWrittenIn int64
	bytesReadOut   int64
	err            atomic.Value

	dataReady   chan token
	bufferReady chan token
}

type Reader struct{ *pipe }
type Writer struct{ *pipe }

func (r Reader) Read(buf []byte) (n int, err error)        { return r.read(buf) }
func (r Reader) WriteTo(w io.Writer) (n int64, err error)  { return r.writeTo(w) }
func (r Reader) Close() error                              { return r.close() }
func (w Writer) Write(data []byte) (n int, err error)      { return w.write(data) }
func (w Writer) ReadFrom(r io.Reader) (n int64, err error) { return w.readFrom(r) }
func (w Writer) Close() error                              { return w.close() }

func (b *pipe) close() error {
	err, _ := b.err.Load().(error)
	ret := err
	if err == nil || err == io.EOF {
		err = io.EOF
		ret = nil
	}
	select {
	case b.bufferReady <- token{}:
	default:
	}
	select {
	case b.dataReady <- token{}:
	default:
	}
	return ret
}

func (b *pipe) write(p []byte) (n int, err error) {
	N := len(p)
	for n < N && err == nil {
		var chunk1, chunk2 []byte
		chunk1, chunk2, err = b.getEmptyChunks()
		nn := copy(chunk1, p[n:])
		if nn == len(chunk1) && n+nn < N {
			nn += copy(chunk2, p[n+nn:])
		}
		n += nn
		b.commitWrite(nn, nil)
	}
	return n, err
}

func (b *pipe) read(p []byte) (n int, err error) {
	data1, data2, err := b.getDataChunks()
	n = copy(p, data1)
	if n == len(data1) && n < len(p) {
		n += copy(p[n:], data2)
	}
	b.commitRead(n, nil)
	return n, err
}

func (b *pipe) readFrom(r io.Reader) (n int64, err error) {
	var chunk1, chunk2 []byte
all:
	for {
		chunk1, chunk2, err = b.getEmptyChunks()
		if err != nil {
			break
		}
		for len(chunk1) > 0 {
			nn, err := r.Read(chunk1)
			b.commitWrite(nn, err)
			n += int64(nn)
			if err != nil {
				break all
			}
			chunk1 = chunk1[nn:]
		}
		for len(chunk2) > 0 {
			nn, err := r.Read(chunk2)
			b.commitWrite(nn, err)
			n += int64(nn)
			if err != nil {
				break all
			}
			chunk2 = chunk2[nn:]
		}
	}
	if err == io.EOF {
		err = nil
	}
	return int64(n), err
}

func (b *pipe) writeTo(w io.Writer) (n int64, err error) {
	var data1, data2 []byte
all:
	for {
		data1, data2, err = b.getDataChunks()
		if err != nil {
			break
		}
		for len(data1) > 0 {
			nn, err := w.Write(data1)
			b.commitRead(nn, err)
			n += int64(nn)
			if err != nil {
				break all
			}
			data1 = data1[nn:]
		}
		for len(data2) > 0 {
			nn, err := w.Write(data2)
			b.commitRead(nn, err)
			n += int64(nn)
			if err != nil {
				break all
			}
			data2 = data2[nn:]
		}
	}
	if err == io.EOF {
		err = nil
	}
	return n, err
}

func (b *pipe) getDataChunks() (data1, data2 []byte, err error) {
	read := atomic.LoadInt64(&b.bytesReadOut)
	for i := 0; ; i++ {
		written := atomic.LoadInt64(&b.bytesWrittenIn)
		empty := written == read
		if !empty {
			data1, data2 = getCircularBufferChunks(b.Buf, read, written)
			return data1, data2, nil
		} else if i < 16 {
			runtime.Gosched()
		} else if err, _ = b.err.Load().(error); err != nil {
			return nil, nil, err
		} else {
			<-b.dataReady
		}
	}
}

func (b *pipe) getEmptyChunks() (chunk1, chunk2 []byte, err error) {
	N := int64(len(b.Buf))
	written := atomic.LoadInt64(&b.bytesWrittenIn)
	for i := 0; ; i++ {
		read := atomic.LoadInt64(&b.bytesReadOut)
		full := (written - read) == N
		if !full {
			chunk1, chunk2 = getCircularBufferChunks(b.Buf, written, read+N)
			return chunk1, chunk2, nil
		} else if i < 16 {
			runtime.Gosched()
		} else if err, _ = b.err.Load().(error); err != nil {
			return nil, nil, err
		} else {
			<-b.bufferReady
		}
	}
}

func (b *pipe) commitWrite(nn int, err error) {
	atomic.AddInt64(&b.bytesWrittenIn, int64(nn))
	if err != nil {
		b.err.Store(err)
	}
	select {
	case b.dataReady <- token{}:
	default:
	}
}

func (b *pipe) commitRead(n int, err error) {
	atomic.AddInt64(&b.bytesReadOut, int64(n))
	if err != nil {
		b.err.Store(err)
	}
	select {
	case b.bufferReady <- token{}:
	default:
	}
}

func getCircularBufferChunks(buf []byte, from, to int64) (first, second []byte) {
	N := int64(len(buf))

	// Empty!
	if from == to {
		panic(fmt.Errorf("Empty buffers requested!  "+
			"From:%d To:%d Delta:%d N:%d", from, to, to-from, N))
	} else if to-from > N {
		panic(fmt.Errorf("More data is in the buffer than the buffer size!  "+
			"From:%d To:%d Delta:%d N:%d", from, to, to-from, N))
	}

	start, end := from%N, to%N
	if start < end {
		// [    S------E     ]
		return buf[start:end], nil
	}
	// [----E       S----]
	// [--------ES-------]
	return buf[start:], buf[:end]
}
