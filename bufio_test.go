package bufioprop

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"testing"
)

// Big random test data.
var testData = random(128 * 1024 * 1024)

// Random generates a pseudo-random binary blob.
func random(length int) []byte {
	src := rand.NewSource(0)

	data := make([]byte, length)
	for i := 0; i < length; i++ {
		data[i] = byte(src.Int63() & 0xff)
	}
	return data
}

// Tests of various buffer sizes to catch index errors.
func TestCopyBuffer3333(t *testing.T) {
	testCopy(3333, t)
}

func TestCopyBuffer33333(t *testing.T) {
	testCopy(33333, t)
}

func TestCopyBuffer333333(t *testing.T) {
	testCopy(333333, t)
}

// Tests that a simple copy works
func testCopy(buffer int, t *testing.T) {
	rb := bytes.NewBuffer(testData)
	wb := new(bytes.Buffer)

	if n, err := Copy(wb, rb, buffer); err != nil { // weird buffer size to catch index bugs
		t.Fatalf("failed to copy data: %v.", err)
	} else if int(n) != len(testData) {
		t.Fatalf("data length mismatch: have %d, want %d.", n, len(testData))
	}
	if bytes.Compare(testData, wb.Bytes()) != 0 {
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
