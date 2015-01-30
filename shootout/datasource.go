package main

import (
	"io"
	"math/rand"
)

// Random generates a pseudo-random binary blob.
func random(length int) []byte {
	src := rand.NewSource(0)

	data := make([]byte, length)
	for i := 0; i < length; i++ {
		data[i] = byte(src.Int63() & 0xff)
	}
	return data
}

type replicatedDataReader struct {
	p     int64
	limit int64
	data  []byte
}

// dataReader returns a reader that returns the given
// data, replicated, returning a maximum of count
// bytes before it returns EOF.
func dataReader(count int64, data []byte) io.Reader {
	// For efficiency, even if the data is small,
	// we make it big enough that we can copy it
	// in decent size chunks.
	chunk := 1024 * 1024
	buf := data[0:len(data):len(data)]
	for len(buf) < chunk {
		buf = append(buf, data...)
	}
	buf = append(buf, buf...)
	buf = buf[0 : len(buf)/2]
	return &replicatedDataReader{
		limit: count,
		data:  buf,
	}
}

func (r *replicatedDataReader) Read(buf []byte) (int, error) {
	nr := 0
	for len(buf) > 0 {
		remain := r.limit - r.p
		if remain <= 0 {
			return nr, io.EOF
		}
		need := len(buf)
		if int(remain) < need {
			need = int(remain)
		}
		if need > len(r.data) {
			need = len(r.data)
		}
		off := int(r.p % int64(len(r.data)))
		n := copy(buf, r.data[off:off+need])
		buf = buf[n:]
		nr += n
		r.p += int64(n)
	}
	return nr, nil
}
