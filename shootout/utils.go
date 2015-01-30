package main

import (
	"math/rand"
	"runtime"
	"time"
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

type Measurement struct {
	Duration time.Duration
	Allocs   uint64
	Bytes    uint64
}

func (m *Measurement) Throughput(size int64) float64 {
	return float64(size) / (1024 * 1024) / m.Duration.Seconds()
}

type Checkpoint struct {
	Time  time.Time
	Stats runtime.MemStats
	temp  runtime.MemStats
}

func (c *Checkpoint) update() {
	runtime.ReadMemStats(&c.Stats)
	c.Time = time.Now()
}

func (c *Checkpoint) ResetTime() {
	c.Time = time.Now()
}

func (c *Checkpoint) Measure() Measurement {
	runtime.GC() // clean up after yourself

	duration := time.Since(c.Time)
	runtime.ReadMemStats(&c.temp)

	return Measurement{
		Duration: duration,
		Allocs:   c.temp.Mallocs - c.Stats.Mallocs,
		Bytes:    c.temp.TotalAlloc - c.Stats.TotalAlloc,
	}
}

func NewCheckpoint() (c Checkpoint) {
	runtime.GC()
	c.update()
	return c
}
