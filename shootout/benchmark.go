package main

import (
	"bytes"
	"io/ioutil"
	"time"
)

// Benchmark runs a high throughput copy to see how implementations compete if
// not rate limited.
func benchmark(data []byte, buffers []int, copier contender) []time.Duration {
	times := []time.Duration{}

	// Simulate the benchmark for every buffer size
	for _, buffer := range buffers {
		source := bytes.NewBuffer(data)

		start := time.Now()
		copier.Copy(ioutil.Discard, source, buffer)
		times = append(times, time.Since(start))
	}
	return times
}
