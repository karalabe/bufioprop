package augustoroman

import (
	"io"
	"sync"
)

func Copy(dst io.Writer, src io.Reader, bufferSize int) (int64, error) {
	pipe := NewBufferedPipe(make([]byte, bufferSize))
	errs := make(chan error, 1)
	go func() {
		_, err := pipe.ReadFrom(src)
		if err != nil {
			errs <- err
		}
		pipe.Close()
	}()
	n, err := pipe.WriteTo(dst)
	errs <- err
	return n, <-errs // Return first error that came up.
}

type BufferedPipe struct {
	Buf []byte

	mutex            sync.Mutex
	bytesWrittenIn   int
	bytesReadOut     int
	err              error
	dataReady        sync.Cond
	buffersAvailable sync.Cond
}

func NewBufferedPipe(buf []byte) *BufferedPipe {
	b := BufferedPipe{Buf: buf}
	b.dataReady.L = &b.mutex
	b.buffersAvailable.L = &b.mutex
	return &b
}

func (b *BufferedPipe) Close() error {
	b.mutex.Lock()
	ret := b.err
	if b.err == nil || b.err == io.EOF {
		b.err = io.EOF
		ret = nil
	}
	b.mutex.Unlock()
	b.dataReady.Signal()
	b.buffersAvailable.Signal()
	return ret
}

func (b *BufferedPipe) Write(p []byte) (n int, err error) {
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

func (b *BufferedPipe) Read(p []byte) (n int, err error) {
	data1, data2, err := b.getDataChunks()
	n = copy(p, data1)
	if n == len(data1) && n < len(p) {
		n += copy(p[n:], data2)
	}
	b.commitRead(n, nil)
	return n, err
}

func (b *BufferedPipe) ReadFrom(r io.Reader) (n int64, err error) {
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

func (b *BufferedPipe) WriteTo(w io.Writer) (n int64, err error) {
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

func (b *BufferedPipe) getDataChunks() (data1, data2 []byte, err error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	for {
		N := len(b.Buf)
		nextRead, nextWrite := b.bytesReadOut%N, b.bytesWrittenIn%N
		full := (b.bytesWrittenIn - b.bytesReadOut) == N
		if nextRead < nextWrite {
			// [    R------W     ]
			return b.Buf[nextRead:nextWrite], nil, nil
		} else if nextRead > nextWrite {
			// [----W       R----]
			return b.Buf[nextRead:], b.Buf[:nextWrite], nil
		} else if full {
			// [--------R/W------] FULL
			return b.Buf[nextRead:], b.Buf[:nextWrite], nil
		} else if b.err != nil {
			return nil, nil, b.err
		}
		// [        R/W      ] !FULL
		// Wait for data to be available or the buffer to be closed.
		b.dataReady.Wait()
	}
}

func (b *BufferedPipe) getEmptyChunks() (chunk1, chunk2 []byte, err error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	for {
		N := len(b.Buf)
		nextRead, nextWrite := b.bytesReadOut%N, b.bytesWrittenIn%N
		empty := b.bytesWrittenIn == b.bytesReadOut
		if nextRead < nextWrite {
			// [    R------W     ]
			return b.Buf[nextWrite:], b.Buf[:nextRead], nil
		} else if nextRead > nextWrite {
			// [----W       R----]
			return b.Buf[nextWrite:nextRead], nil, nil
		} else if empty {
			// [        R/W      ] EMPTY
			return b.Buf[nextWrite:], b.Buf[:nextRead], nil
		} else if b.err != nil {
			return nil, nil, b.err
		}
		// [--------R/W------] FULL (== !EMPTY)
		// Wait for buffer space to be available or the buffer to be closed.
		b.buffersAvailable.Wait()
	}
}

func (b *BufferedPipe) commitWrite(nn int, err error) {
	b.mutex.Lock()
	if b.err == nil {
		b.err = err
	}
	b.bytesWrittenIn += nn
	b.mutex.Unlock()
	b.dataReady.Signal()
}

func (b *BufferedPipe) commitRead(n int, err error) {
	b.mutex.Lock()
	if b.err == nil {
		b.err = err
	}
	b.bytesReadOut += n
	b.mutex.Unlock()
	b.buffersAvailable.Signal()
}
