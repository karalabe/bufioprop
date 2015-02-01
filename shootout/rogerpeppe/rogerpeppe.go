package rogerpeppe

import (
	"io"
	"github.com/karalabe/bufioprop/shootout/rogerpeppe/bufpipe"
)

func IOCopy(w io.Writer, r io.Reader, size int) (int64, error) {
	pr, pw := io.Pipe()
	done := make(chan error)
	go func() {
		_, err := io.Copy(pw, r)
		pw.Close()
		done <- err
	}()
	n, err0 := io.Copy(w, pr)
	err1 := <-done
	if err0 != nil {
		return n, err0
	}
	return n, err1
}


func Copy(w io.Writer, r io.Reader, size int) (int64, error) {
	pr, pw := bufpipe.New(size)
	done := make(chan error)
	go func() {
		_, err := io.Copy(pw, r)
		pw.Close()
		done <- err
	}()
	n, err0 := io.Copy(w, pr)
	err1 := <-done
	if err0 != nil {
		return n, err0
	}
	return n, err1
}
