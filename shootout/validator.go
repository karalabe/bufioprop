package main

import (
	"bytes"
	"fmt"
)

// Test verifies that an implementation works correctly under high load.
func test(data []byte, copier contender) (result bool) {
	// Short circuit if manually disabled (probably an unrecoverable panic)
	if copier.Disable {
		fmt.Printf("%15s: unrecoverable panic, disabled.\n", copier.Name)
		return false
	}
	// Make sure a panic doesn't kill the shootout
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%15s: panic.\n", copier.Name)
			result = false
		}
	}()
	// Do a full speed copy to catch threading bugs
	rb := bytes.NewBuffer(data)
	wb := new(bytes.Buffer)

	if n, err := copier.Copy(wb, rb, 333333); err != nil { // weird buffer size to catch index bugs
		fmt.Printf("%15s: failed to copy data: %v.\n", copier.Name, err)
		return false
	} else if int(n) != len(data) {
		fmt.Printf("%15s: data length mismatch: have %d, want %d.\n", copier.Name, n, len(data))
		return false
	}
	if bytes.Compare(data, wb.Bytes()) != 0 {
		fmt.Printf("%15s: corrupt data on the output.\n", copier.Name)
		return false
	}
	fmt.Printf("%15s: test passed.\n", copier.Name)
	return true
}
