package main

import "math/rand"

// Random generates a pseudo-random binary blob.
func random(length int) []byte {
	src := rand.NewSource(0)

	data := make([]byte, length)
	for i := 0; i < length; i++ {
		data[i] = byte(src.Int63() & 0xff)
	}
	return data
}
