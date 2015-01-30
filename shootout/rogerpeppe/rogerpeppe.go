package rogerpeppe

import (
	"io"

	"github.com/karalabe/bufioprop/shootout/rogerpeppe/bufpipe"
)

func Copy(w io.Writer, r io.Reader, buffer int) (int64, error) {
	pr, pw := bufpipe.New(buffer)
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
