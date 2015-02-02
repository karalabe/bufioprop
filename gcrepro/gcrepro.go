package main

import (
	"bytes"
	"fmt"
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

	fmt.Printf("Gomaxprocs %d, allocs: %d, bytes: %d\n", runtime.GOMAXPROCS(0), end.Mallocs-start.Mallocs, end.TotalAlloc-start.TotalAlloc)
}
