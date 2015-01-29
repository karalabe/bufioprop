package main

import (
	"bytes"
	"fmt"
	"time"
)

// Benchmark runs a high throughput copy to see how implementations compete if
// not rate limited.
func benchmark(data []byte, bufs []int, copier contender) {
	for _, buf := range bufs {
		rb := bytes.NewBuffer(data)
		wb := new(bytes.Buffer)

		start := time.Now()
		copier.Copy(wb, rb, buf)
		fmt.Printf("%15s: data %dMB, buffer %8dB time %v.\n", copier.Name, len(data)/1024/1024, buf, time.Since(start))
	}
}
