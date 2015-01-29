package main

import (
	"bytes"
	"fmt"
	"time"

	"github.com/olekukonko/tablewriter"
)

// Benchmark runs a high throughput copy to see how implementations compete if
// not rate limited.
func benchmark(data []byte, bufs []int, copier contender, table *tablewriter.Table) {
	row := []string{copier.Name}
	for _, buf := range bufs {
		rb := bytes.NewBuffer(data)
		wb := new(bytes.Buffer)

		start := time.Now()
		copier.Copy(wb, rb, buf)
		row = append(row, fmt.Sprintf("%v", time.Since(start)))
	}
	table.Append(row)
}
