package jnml

import (
	"io"
)

func Copy(dst io.Writer, src io.Reader, buffer int) (int64, error) {
	const page = 1 << 12

	w := make(chan []byte, 1000)
	r := make(chan interface{}, 1000)

	go func() {
		for chunk := range w {
			nn := len(chunk)
			for len(chunk) != 0 {
				n, err := dst.Write(chunk)
				if err != nil {
					r <- err
					return
				}
				chunk = chunk[n:]
			}
			r <- nn
		}
		r <- nil
	}()

	if buffer < page {
		buffer = page
	}
	var nn int64
	for {
		for i := 0; buffer < page || i == 0; i++ {
			select {
			case x := <-r:
				switch x := x.(type) {
				case error:
					return nn, x
				case int:
					buffer += x
				}
			default:
			}
		}

		b := make([]byte, page)
		n, err := src.Read(b)
		if n != 0 {
			nn += int64(n)
			w <- b[:n]
			buffer -= n
		}

		if err != nil {
			if err == io.EOF {
				close(w)
				for {
					switch x := (<-r).(type) {
					case nil:
						return nn, nil
					case error:
						return nn, x
					}
				}
			}

			return nn, err
		}
	}
}
