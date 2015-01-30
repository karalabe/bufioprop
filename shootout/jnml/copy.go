package jnml

import (
	"io"
)

func Copy(dst io.Writer, src io.Reader, buffer int) (int64, error) {
	type t struct {
		err error
		b   []byte
	}
	page := 1 << 20
	if buffer < page {
		page = buffer
	}
	w := make(chan []byte, buffer/page+2)
	r := make(chan t, buffer/page+2)
	go func() {
		for chunk := range w {
			c0 := chunk
			for len(chunk) != 0 {
				n, err := dst.Write(chunk)
				if err != nil {
					r <- t{err:err}
					return
				}
				chunk = chunk[n:]
			}
			r <- t{b: c0[:page]}
		}
		r <- t{}
	}()
	var nn int64
	pages := [][]byte{make([]byte, page)}
	for {
		var b []byte
		switch n := len(pages); n {
		case 0:
			b = make([]byte, page)
		default:
			b = pages[n-1]
			pages[n-1] = nil
			pages = pages[:n-1]
		}
		n, err := src.Read(b)
		if n != 0 {
			nn += int64(n)
			w <- b[:n]
			buffer -= page
		}
		if err != nil {
			close(w)
			if err == io.EOF {
				for {
					x := <-r
					if x.b != nil {
						continue
					}
					return nn, x.err
				}
			}
			return nn, err
		}
		for buffer < page {
			x := <-r
			if b = x.b; b != nil {
				buffer += page
				pages = append(pages, b)
				break
			}
			return nn, x.err
		}
	}
}
