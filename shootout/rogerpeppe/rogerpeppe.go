package rogerpeppe

import (
	"github.com/karalabe/bufioprop/shootout/bufpipe"
	"io"
)

func Copy(w io.Writer, r io.Reader, ignored int) (int64, error) {
	pr, pw := bufpipe.New(1024 * 1024)
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
