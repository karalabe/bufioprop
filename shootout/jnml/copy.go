package jnml

import (
	"io"
)

func Copy(dst io.Writer, src io.Reader, buffer int) (int64, error) {
	page := 1 << 12
	if buffer < page {
		page = buffer
	}
	w := make(chan []byte, buffer/page+2)
	r := make(chan interface{}, buffer/page+2)
	go func() {
		for chunk := range w {
			c0 := chunk
			for len(chunk) != 0 {
				n, err := dst.Write(chunk)
				if err != nil {
					r <- err
					return
				}
				chunk = chunk[n:]
			}
			r <- c0[:page]
		}
		r <- nil
	}()
	var nn int64
	var pages [][]byte
	for {
		for buffer < page {
			select {
			case x := <-r:
				switch x := x.(type) {
				case error:
					close(w)
					return nn, x
				case []byte:
					buffer += page
					pages = append(pages, x)
				}
			}
		}
		select {
		case x := <-r:
			switch x := x.(type) {
			case error:
				close(w)
				return nn, x
			case []byte:
				buffer += page
				pages = append(pages, x)
			}
		default:
		}
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
