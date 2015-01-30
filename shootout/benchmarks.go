package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"time"
)

// BenchmarkLatency measures the amount of time it takes for one single byte to
// propagate through the copy.
func benchmarkLatency(iters int, copier contender) {
	ir, iw := io.Pipe()
	or, ow := io.Pipe()

	// Start the copy and push a few values through to initialize internals
	go copier.Copy(ow, ir, 1024)

	input, output := []byte{0xff}, make([]byte, 1)
	for i := 0; i < iters; i++ {
		iw.Write(input)
		or.Read(output)
	}
	// Do the same thing, but time it this time
	start := time.Now()
	for i := 0; i < iters; i++ {
		iw.Write(input)
		or.Read(output)
	}
	ow.Close()

	fmt.Printf("%15s: latency %v.\n", copier.Name, time.Since(start)/time.Duration(iters))
}

// BenchmarkThroughput runs a high throughput copy to see how implementations compete if
// not rate limited.
func benchmarkThroughput(data []byte, buffers []int, copier contender) (results []Measurement) {
	// Simulate the benchmark for every buffer size
	for _, buffer := range buffers {
		source := bytes.NewBuffer(data)

		c := NewCheckpoint()
		copier.Copy(ioutil.Discard, source, buffer)
		m := c.Measure()

		results = append(results, m)
	}
	return results
}
