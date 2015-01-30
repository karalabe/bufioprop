package jnml

import "io"

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
					r <- t{err: err}
					return
				}
				chunk = chunk[n:]
			}
			r <- t{b: c0[:page]}
		}
		r <- t{}
	}()
	pages := make([][]byte, buffer/page)
	buf := make([]byte, buffer)
	for i := range pages {
		pages[i] = buf[i*page : i*page+page]
	}
	for nn := int64(0); ; {
		b := pages[len(pages)-1]
		pages = pages[:len(pages)-1]
		n, err := src.Read(b)
		if n != 0 {
			nn += int64(n)
			w <- b[:n]
			buffer -= page
		}
		var x t
		if err != nil {
			close(w)
			if err != io.EOF {
				return nn, err
			}
			for x = <-r; x.b != nil; x = <-r {
			}
			return nn, x.err
		}
		for buffer < page {
			if x = <-r; x.b == nil {
				return nn, x.err
			}
			buffer += page
			pages = append(pages, b)
		}
	}
}
