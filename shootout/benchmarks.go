package main

import (
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

	c := NewCheckpoint()
	input, output := []byte{0xff}, make([]byte, 1)
	for i := 0; i < iters; i++ {
		iw.Write(input)
		or.Read(output)
	}
	// Do the same thing, but time it this time
	c.ResetTime()
	for i := 0; i < iters; i++ {
		iw.Write(input)
		or.Read(output)
	}
	ow.Close()
	m := c.Measure()

	fmt.Printf("%20s: %7v %7d allocs %9d B.\n", copier.Name, m.Duration/time.Duration(iters), m.Allocs, m.Bytes)
}

// BenchmarkThroughput runs a high throughput copy to see how implementations compete if
// not rate limited.
func benchmarkThroughput(count int64, data []byte, buffers []int, copier contender) (results []Measurement) {
	// Simulate the benchmark for every buffer size, keep the best out of three
	for _, buffer := range buffers {
		var best Measurement

		for i := 0; i < 3; i++ {
			source := dataReader(count, data)

			c := NewCheckpoint()
			copier.Copy(ioutil.Discard, source, buffer)
			m := c.Measure()

			if i == 0 || m.Duration < best.Duration {
				best = m
			}
		}
		results = append(results, best)
	}
	return results
}
