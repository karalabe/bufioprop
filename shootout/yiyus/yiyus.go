package yiyus

import (
	"bytes"
	"io"
)

func Copy(w io.Writer, r io.Reader, ignored int) (int64, error) {
	b := bytes.NewBuffer([]byte{})
	errc := make(chan error)
	nc := make(chan int64)
	go func() {
		n, err := io.Copy(b, r)
		nc <- n
		errc <- err
	}()
	go func() {
		n, err := io.Copy(w, b)
		nc <- n
		errc <- err
	}()
	n, err := <-nc, <-errc
	if err == nil {
		n, err = <-nc, <-errc
	}
	return n, err
}
