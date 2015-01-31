package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/karalabe/bufioprop"
	"github.com/karalabe/bufioprop/shootout/augustoroman"
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
	{"rogerpeppe.IOCopy", rogerpeppe.IOCopy, ""},
	{"mattharden.Copy", mattharden.Copy, ""},
	{"yiyus.Copy", yiyus.Copy, ""},
	{"egonelbre.Copy", egonelbre.Copy, ""},
	{"jnml.Copy", jnml.Copy, ""},
	{"ncw.Copy", ncw.Copy, "deadlock in latency benchmark"},
	{"bakulshah.Copy", bakulshah.Copy, ""},
	{"augustoroman.Copy", augustoroman.Copy, ""},
}

func main() {
	// Run on multiple threads to catch race bugs
	runtime.GOMAXPROCS(8)

	// Collect the shot out implementations
	failed := make(map[string]struct{})

	fmt.Println("Manually disabled contenders:")
	for _, copier := range contenders {
		if len(copier.Disable) != 0 {
			fmt.Printf("%20s: %s.\n", copier.Name, copier.Disable)
			failed[copier.Name] = struct{}{}
		}
	}
	fmt.Println("------------------------------------------------\n")

	// Run a batch of tests to make sure the function works
	fmt.Println("High throughput tests:")

	count := int64(128 * 1024 * 1024)
	data := random(1024 * 1024)
	for _, copier := range contenders {
		if _, ok := failed[copier.Name]; !ok {
			if !test(count, data, copier) {
				failed[copier.Name] = struct{}{}
			}
		}
	}
	fmt.Println("------------------------------------------------\n")

	// Simulate copying between various types of readers and writers
	count = 32 * 1024 * 1024

	fmt.Println("Stable input, stable output shootout:")
	for _, copier := range contenders {
		if _, ok := failed[copier.Name]; !ok {
			in, out := stableInput(count, data), stableOutput()
			if res := shootout(in, out, count, copier); res < 8 {
				failed[copier.Name] = struct{}{}
			}
		}
	}
	fmt.Println("\nStable input, bursty output shootout:")
	for _, copier := range contenders {
		if _, ok := failed[copier.Name]; !ok {
			in, out := stableInput(count, data), burstyOutput()
			if res := shootout(in, out, count, copier); res < 8 {
				failed[copier.Name] = struct{}{}
			}
		}
	}
	fmt.Println("\nBursty input, stable output shootout:")
	for _, copier := range contenders {
		if _, ok := failed[copier.Name]; !ok {
			in, out := burstyInput(count, data), stableOutput()
			if res := shootout(in, out, count, copier); res < 8 {
				failed[copier.Name] = struct{}{}
			}
		}
	}
	fmt.Println("------------------------------------------------")

	// Run various benchmarks of the remaining contenders
	count = 256 * 1024 * 1024
	procs := []int{1, 8}
	buffers := []int{333, 4*1024 + 59, 64*1024 - 177, 1024*1024 - 17, 16*1024*1024 + 85}

	for _, proc := range procs {
		runtime.GOMAXPROCS(proc)

		fmt.Printf("\nLatency benchmarks (GOMAXPROCS = %d):\n", runtime.GOMAXPROCS(0))
		for _, copier := range contenders {
			if _, ok := failed[copier.Name]; !ok {
				benchmarkLatency(1000000, copier)
			}
		}
	}

	for _, proc := range procs {
		runtime.GOMAXPROCS(proc)

		fmt.Printf("\nThroughput (GOMAXPROCS = %d) (%d MB):\n", proc, count/1024/1024)

		type Result struct {
			Name    string
			Results []Measurement
		}

		results := make([]Result, 0, len(contenders))
		for _, copier := range contenders {
			if _, ok := failed[copier.Name]; !ok {
				res := benchmarkThroughput(count, data, buffers, copier)
				results = append(results, Result{copier.Name, res})
			}
		}

		type formatter func(m Measurement) string
		table := func(title string, format formatter) {
			table := tablewriter.NewWriter(os.Stdout)
			header := []string{title}
			for _, buf := range buffers {
				header = append(header, strconv.Itoa(buf))
			}
			table.SetHeader(header)
			for _, r := range results {
				row := []string{r.Name}
				for _, res := range r.Results {
					row = append(row, format(res))
				}
				table.Append(row)
			}
			table.Render()
		}

		fmt.Println()
		table("Throughput", func(m Measurement) string {
			return fmt.Sprintf("%5.2f", m.Throughput(count))
		})
		fmt.Println()

		table("Allocs/Bytes", func(m Measurement) string {
			return fmt.Sprintf("(%8d / %8d)", m.Allocs, m.Bytes)
		})
	}
}

// Shootout runs a copy operation on the given input/output endpoints with the
// specified copy function.
func shootout(r io.Reader, w io.Writer, size int64, copier contender) float64 {
	buffer := 12 * 1024 * 1024

	time.Sleep(time.Millisecond) // why do I need this? why do the data source allocs seep into the checkpoint?

	c := NewCheckpoint()
	if n, err := copier.Copy(w, r, buffer); n != size || err != nil {
		fmt.Printf("%20s: operation failed: have n %d, want n %d, err %v.\n", copier.Name, n, size, err)
		return -1
	}
	m := c.Measure()

	fmt.Printf("%20s: %14v %10f mbps %5d allocs %9d B\n", copier.Name, m.Duration, m.Throughput(size), m.Allocs, m.Bytes)

	return m.Throughput(size)
}

// StableInput creates a 10MBps data source streaming stably in small chunks of
// 100KB each.
func stableInput(count int64, data []byte) io.Reader {
	return input(100*time.Millisecond, 1024*1024, dataReader(count, data))
}

// BurstyInput creates a 10MBps data source streaming in bursts of 10MB.
func burstyInput(count int64, data []byte) io.Reader {
	return input(1000*time.Millisecond, 10*1024*1024, dataReader(count, data))
}

// StableOutput creates a 10MBps data sink consuming stably in small chunks of
// 100KB each.
func stableOutput() io.Writer {
	return output(100*time.Millisecond, 1024*1024)
}

// BurstyOutput creates a 10MBps data sink consuming in bursts of 10MB.
func burstyOutput() io.Writer {
	return output(1000*time.Millisecond, 10*1024*1024)
}

// Input creates an unbuffered data source, filled at the specified rate
// producing count bytes by reading the given source.
func input(cycle time.Duration, chunk int, source io.Reader) io.Reader {
	pr, pw := io.Pipe()

	// Input generator that will produce data at a specified rate
	go func() {
		defer pw.Close()

		buffer := make([]byte, chunk)
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
			}
			if err != nil {
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
	pr, pw := io.Pipe()

	// Output reader that will consume data at a specified rate
	go func() {
		defer pr.Close()

		buffer := make([]byte, chunk)
		for {
			// Consume the next chunk from the output stream
			_, err := io.ReadFull(pr, buffer)
			if err == io.EOF {
				return
			}
			if err != nil {
				panic(err)
			}
			// Sleep a while to simulate throughput
			time.Sleep(cycle)
		}
	}()
	return pw
}
