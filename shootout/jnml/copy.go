package jnml

import "io"

func Copy(dst io.Writer, src io.Reader, buffer int) (int64, error) {
	b := make([]byte, buffer)
	w := make(chan int, 16)
	r := make(chan int, 16)
	var werr error
	go func() {
		var d, i, n, t int
		for q := range w {
			if q < 0 {
				return
			}
			for t += q; t != 0; t -= n {
				if n = i + t; n > buffer {
					n = buffer
				}
				var err error
				if n, err = dst.Write(b[i:n]); err != nil {
					werr = err
					r <- -1
					return
				}
				if i += n; i == buffer {
					i = 0
				}
				d += n
				select {
				case r <- d:
					d = 0
				default:
				}
				select {
				case q = <-w:
					if q < 0 {
						return
					}
					t += q
				default:
				}
			}
		}
		r <- 0
	}()
	var i, j, p, q int
	var nn int64
	var s []byte
	for {
		switch {
		case p == 0:
			s = b[i:buffer]
		case i == j:
			n := <-r
			if n == -1 {
				return nn, werr
			}
			p -= n
			if j += n; j >= buffer {
				j -= buffer
				continue
			}
		case i > j:
			s = b[i:buffer]
		default:
			s = b[i:j]
		}
		n, err := src.Read(s)
		if n != 0 {
			q += n
			select {
			case w <- q:
				q = 0
			default:
			}
			p += n
			if i += n; i == buffer {
				i = 0
			}
			nn += int64(n)
			select {
			case n = <-r:
				if n < 0 {
					return nn, werr
				}
				p -= n
				if j += n; j >= buffer {
					j -= buffer
					continue
				}
			default:
			}
		}
		if err != nil {
			if err != io.EOF {
				w <- -1
				return nn, err
			}
			if q != 0 {
				w <- q
			}
			close(w)
			for n = <-r; n > 0; n = <-r {
			}
			return nn, werr
		}
	}
}
