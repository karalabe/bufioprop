package mattharden

import (
	"bufio"
	"io"
)

func Copy(w io.Writer, r io.Reader, buflen int) (int64, error) {
	return bufcopy(w, r, buflen/16, 16)
}

func bufcopy(w io.Writer, r io.Reader, buflen int, count int) (int64, error) {
	c_in := make(chan []byte, count)
	c_out := make(chan []byte)
	c_err := make(chan error)
	for i := 0; i < count; i++ {
		c_in <- make([]byte, buflen)
	}
	go func() {
		var err error
		for err == nil {
			buf := <-c_in
			var n int
			n, err = r.Read(buf)
			buf = buf[:n]
			c_out <- buf
		}
		if err == io.EOF {
			err = nil
		}
		c_err <- err
	}()
	var length int64
	for {
		select {
		case err := <-c_err:
			return length, err
		case buf := <-c_out:
			n, err := w.Write(buf)
			length += int64(n)
			if err != nil {
				return length, err
			}
			c_in <- buf[:cap(buf)]
		}
	}
}
