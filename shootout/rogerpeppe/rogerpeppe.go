package rogerpeppe

import (
	"io"
	"os"
)

func Copy(w io.Writer, r io.Reader, ignored int) (int64, error) {
	pr, pw, err := os.Pipe()
	if err != nil {
		return 0, err
	}
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
