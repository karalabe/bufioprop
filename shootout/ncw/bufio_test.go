package ncw

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"testing"
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

// Tests that a simple copy works
func TestCopy(t *testing.T) {
	data := random(128 * 1024 * 1024)

	rb := bytes.NewBuffer(data)
	wb := new(bytes.Buffer)

	if n, err := Copy(wb, rb, 333333); err != nil { // weird buffer size to catch index bugs
		t.Fatalf("failed to copy data: %v.", err)
	} else if int(n) != len(data) {
		t.Fatalf("data length mismatch: have %d, want %d.", n, len(data))
	}
	if bytes.Compare(data, wb.Bytes()) != 0 {
		t.Errorf("copy did not work properly.")
	}
}

// Various combinations of benchmarks to measure the copy.
func BenchmarkCopy1KbData1KbBuffer(b *testing.B) {
	benchmarkCopy(1024, 1024, b)
}

func BenchmarkCopy1KbData128KbBuffer(b *testing.B) {
	benchmarkCopy(1024, 128*1024, b)
}

func BenchmarkCopy1KbData1MbBuffer(b *testing.B) {
	benchmarkCopy(1024, 1024*1024, b)
}

func BenchmarkCopy1MbData1KbBuffer(b *testing.B) {
	benchmarkCopy(1024*1024, 1024, b)
}

func BenchmarkCopy1MbData128KbBuffer(b *testing.B) {
	benchmarkCopy(1024*1024, 128*1024, b)
}

func BenchmarkCopy1MbData1MbBuffer(b *testing.B) {
	benchmarkCopy(1024*1024, 1024*1024, b)
}

func BenchmarkCopy128MbData1KbBuffer(b *testing.B) {
	benchmarkCopy(128*1024*1024, 1024, b)
}

func BenchmarkCopy128MbData128KbBuffer(b *testing.B) {
	benchmarkCopy(128*1024*1024, 128*1024, b)
}

func BenchmarkCopy128MbData1MbBuffer(b *testing.B) {
	benchmarkCopy(128*1024*1024, 1024*1024, b)
}

// BenchmarkCopy measures the performance of the buffered copying for a given
// buffer size.
func benchmarkCopy(data int, buffer int, b *testing.B) {
	blob := random(data)

	b.SetBytes(int64(data))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		Copy(ioutil.Discard, bytes.NewBuffer(blob), buffer)
	}
}
