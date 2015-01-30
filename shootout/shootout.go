package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/karalabe/bufioprop"
	"github.com/karalabe/bufioprop/shootout/bakulshah"
	"github.com/karalabe/bufioprop/shootout/egonelbre"
	"github.com/karalabe/bufioprop/shootout/jnml"
	"github.com/karalabe/bufioprop/shootout/mattharden"
	"github.com/karalabe/bufioprop/shootout/ncw"
	"github.com/karalabe/bufioprop/shootout/rogerpeppe"
	"github.com/karalabe/bufioprop/shootout/yiyus"
	"github.com/olekukonko/tablewriter"
)

type copyFunc func(dst io.Writer, src io.Reader, buffer int) (int64, error)

type contender struct {
	Name    string
	Copy    copyFunc
	Disable string
}

var contenders = []contender{
	// First contender is the build in io.Copy (wrapped in out specific signature)
	{"io.Copy", func(dst io.Writer, src io.Reader, buffer int) (int64, error) {
		return io.Copy(dst, src)
	}, ""},
	// Second contender is the proposed bufio.Copy (currently at bufioprop.Copy)
	{"[!] bufio.Copy", bufioprop.Copy, ""},

	// Other contenders written by mailing list contributions
	{"rogerpeppe.Copy", rogerpeppe.Copy, ""},
	{"mattharden.Copy", mattharden.Copy, ""},
	{"yiyus.Copy", yiyus.Copy, ""},
	{"egonelbre.Copy", egonelbre.Copy, ""},
	{"jnml.Copy", jnml.Copy, ""},
	{"ncw.Copy", ncw.Copy, "deadlock in latency benchmark"},
	{"bakulshah.Copy", bakulshah.Copy, ""},
}

func main() {
	// Run on multiple threads to catch race bugs
	runtime.GOMAXPROCS(8)

	// Collect the shot out implementations
	failed := make(map[string]struct{})

	fmt.Println("Manually disabled contenders:")
	for _, copier := range contenders {
		if len(copier.Disable) != 0 {
			fmt.Printf("%15s: %s.\n", copier.Name, copier.Disable)
			failed[copier.Name] = struct{}{}
		}
	}
	fmt.Println("------------------------------------------------\n")

	// Run a batch of tests to make sure the function works
	fmt.Println("High throughput tests:")

	data := random(128 * 1024 * 1024)
	for _, copier := range contenders {
		if _, ok := failed[copier.Name]; !ok {
			if !test(data, copier) {
				failed[copier.Name] = struct{}{}
			}
		}
	}
	fmt.Println("------------------------------------------------\n")

	// Simulate copying between various types of readers and writers
	data = random(32 * 1024 * 1024)

	fmt.Println("Stable input, stable output shootout:")
	for _, copier := range contenders {
		if _, ok := failed[copier.Name]; !ok {
			in, out := stableInput(data), stableOutput()
			if res := shootout(in, out, len(data), copier); res < 9 {
				failed[copier.Name] = struct{}{}
			}
		}
	}
	fmt.Println("\nStable input, bursty output shootout:")
	for _, copier := range contenders {
		if _, ok := failed[copier.Name]; !ok {
			in, out := stableInput(data), burstyOutput()
			if res := shootout(in, out, len(data), copier); res < 9 {
				failed[copier.Name] = struct{}{}
			}
		}
	}
	fmt.Println("\nBursty input, stable output shootout:")
	for _, copier := range contenders {
		if _, ok := failed[copier.Name]; !ok {
			in, out := burstyInput(data), stableOutput()
			if res := shootout(in, out, len(data), copier); res < 9 {
				failed[copier.Name] = struct{}{}
			}
		}
	}
	fmt.Println("------------------------------------------------\n")

	// Run various benchmarks of the remaining contenders
	fmt.Println("Latency benchmarks:")
	for _, copier := range contenders {
		if _, ok := failed[copier.Name]; !ok {
			benchmarkLatency(1024, copier)
		}
	}
	fmt.Printf("\nThroughput benchmarks (%d MB):\n", len(data)/1024/1024)

	data = random(256 * 1024 * 1024)
	buffers := []int{333, 4*1024 + 59, 64*1024 - 177, 1024*1024 - 17, 16*1024*1024 + 85}

	table := tablewriter.NewWriter(os.Stdout)
	header := []string{"Solution"}
	for _, buf := range buffers {
		header = append(header, "Buf-"+strconv.Itoa(buf))
	}
	table.SetHeader(header)

	for _, copier := range contenders {
		if _, ok := failed[copier.Name]; !ok {
			// Run the benchmark
			results := benchmarkThroughput(data, buffers, copier)

			// Collect and report the results
			row := []string{copier.Name}
			for _, res := range results {
				row = append(row, fmt.Sprintf("%.2f mbps", res))
			}
			table.Append(row)
		}
	}
	table.Render()
}

// Shootout runs a copy operation on the given input/output endpoints with the
// specified copy function.
func shootout(r io.Reader, w io.Writer, size int, copier contender) float64 {
	buffer := 1024 * 1024

	start := time.Now()
	if n, err := copier.Copy(w, r, buffer); int(n) != size || err != nil {
		fmt.Printf("%15s: operation failed: have n %d, want n %d, err %v.\n", copier.Name, n, size, err)
		return -1
	}
	elapsed := time.Since(start)
	throughput := float64(size) / (1024 * 1024) / elapsed.Seconds()
	fmt.Printf("%15s: %14v %10f mbps.\n", copier.Name, elapsed, throughput)

	return throughput
}

// StableInput creates a 10MBps data source streaming stably in small chunks of
// 100KB each.
func stableInput(data []byte) io.Reader {
	return input(10*time.Millisecond, 100*1024, data)
}

// BurstyInput creates a 10MBps data source streaming in bursts of 1MB.
func burstyInput(data []byte) io.Reader {
	return input(100*time.Millisecond, 1000*1024, data)
}

// StableOutput creates a 10MBps data sink consuming stably in small chunks of
// 100KB each.
func stableOutput() io.Writer {
	return output(10*time.Millisecond, 100*1024)
}

// BurstyOutput creates a 10MBps data sink consuming in bursts of 1MB.
func burstyOutput() io.Writer {
	return output(100*time.Millisecond, 1000*1024)
}

// Input creates an unbuffered data source, filled at the specified rate.
func input(cycle time.Duration, chunk int, data []byte) io.Reader {
	source := bytes.NewBuffer(data)
	buffer := make([]byte, chunk)
	pr, pw := io.Pipe()

	// Input generator that will produce data at a specified rate
	go func() {
		defer pw.Close()

		for {
			// Make the next chunk available in the input stream
			n, err := io.ReadFull(source, buffer)
			if n > 0 {
				if _, err := pw.Write(buffer[:n]); err != nil {
					panic(err)
				}
			}
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return
			} else if err != nil {
				panic(err)
			}
			// Sleep a while to simulate throughput
			time.Sleep(cycle)
		}
	}()
	return pr
}

// Input creates an unbuffered data sink, emptied at the specified rate.
func output(cycle time.Duration, chunk int) io.Writer {
	buffer := make([]byte, chunk)
	pr, pw := io.Pipe()

	// Output reader that will consume data at a specified rate
	go func() {
		defer pr.Close()

		for {
			// Consume the next chunk from the output stream
			_, err := io.ReadFull(pr, buffer)
			if err == io.EOF {
				return
			} else if err != nil {
				panic(err)
			}
			// Sleep a while to simulate throughput
			time.Sleep(cycle)
		}
	}()
	return pw
}
