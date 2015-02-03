package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"runtime"

	"github.com/karalabe/bufioprop"

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

func main() {
	data := random(256 * 1024 * 1024)
	run(data, 1)
	run(data, 1)
	run(data, 1)
	fmt.Println()
	run(data, 8)
	run(data, 8)
	run(data, 8)

	fmt.Println()
	fmt.Println()

	iter := 256 * 1024
	burst(iter, 1)
	burst(iter, 1)
	burst(iter, 1)
	fmt.Println()
	burst(iter, 8)
	burst(iter, 8)
	burst(iter, 8)
}

func burst(iters int, threads int) {
	runtime.GOMAXPROCS(threads)

	ir, iw := io.Pipe()
	or, ow := io.Pipe()

	// Gather memory stats
	start := new(runtime.MemStats)
	runtime.ReadMemStats(start)

	// Run the operation
	go bufioprop.Copy(ow, ir, 1024)

	input, output := []byte{0xff}, make([]byte, 1)
	for i := 0; i < iters; i++ {
		iw.Write(input)
		or.Read(output)
	}
	ow.Close()

	// Gather memory stats and report
	end := new(runtime.MemStats)
	runtime.ReadMemStats(end)

	fmt.Printf("short bursts: gomaxprocs %d, allocs: %d, bytes: %d\n", runtime.GOMAXPROCS(0), end.Mallocs-start.Mallocs, end.TotalAlloc-start.TotalAlloc)
}

func run(data []byte, threads int) {
	runtime.GOMAXPROCS(threads)

	// Gather memory stats
	start := new(runtime.MemStats)
	runtime.ReadMemStats(start)

	// Run the operation
	bufioprop.Copy(ioutil.Discard, bytes.NewReader(data), 1024*1024)

	// Gather memory stats and report
	end := new(runtime.MemStats)
	runtime.ReadMemStats(end)

	fmt.Printf("long run: gomaxprocs %d, allocs: %d, bytes: %d\n", runtime.GOMAXPROCS(0), end.Mallocs-start.Mallocs, end.TotalAlloc-start.TotalAlloc)
}
