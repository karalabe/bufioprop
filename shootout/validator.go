package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
)

// Test verifies that an implementation works correctly under high load.
func test(count int64, data []byte, copier contender) (result bool) {
	// Make sure a panic doesn't kill the shootout
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%15s: panic.\n", copier.Name)
			result = false
		}
	}()
	hash1 := sha256.New()
	// Do a full speed copy to catch threading bugs
	r := io.TeeReader(dataReader(count, data), hash1)
	hash2 := sha256.New()

	n, err := copier.Copy(hash2, r, 333333)
	if err != nil { // weird buffer size to catch index bugs
		fmt.Printf("%15s: failed to copy data: %v.\n", copier.Name, err)
		return false
	}
	if n != count {
		fmt.Printf("%15s: data length mismatch: have %d, want %d.\n", copier.Name, n, count)
		return false
	}
	if bytes.Compare(hash1.Sum(nil), hash2.Sum(nil)) != 0 {
		fmt.Printf("%15s: corrupt data on the output.\n", copier.Name)
		return false
	}
	fmt.Printf("%15s: test passed.\n", copier.Name)
	return true
}
