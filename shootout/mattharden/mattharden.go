package mattharden

import (
	"bufio"
	"io"
)

func Copy(w io.Writer, r io.Reader, buflen int) (int64, error) {
	pr, pw := io.Pipe()
	go func() {
		_, err := io.Copy(pw, bufio.NewReaderSize(r, buflen))
		pw.CloseWithError(err)
	}()
	bw := bufio.NewWriterSize(w, buflen)
	n, err := io.Copy(bw, pr)
	if err == nil {
		err = bw.Flush()
	} else {
		// Ignoring error from Flush in favor of the previous error
		bw.Flush()
	}
	return n - int64(bw.Buffered()), err
}
