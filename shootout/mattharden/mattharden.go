package mattharden

import (
	"io"
)

type limitedBuf struct {
        s sema
	r io.Reader
        w io.WriteCloser
}

func (buf *limitedBuf) Read(p []byte) (n int, err error) {
	n, err = buf.r.Read(p)
        buf.s.Add(n)
	return n, err
}

func (buf *limitedBuf) Write(p []byte) (n int, err error) {
	for err == nil && len(p) > 0 {
            n_ := buf.s.Sub(len(p))
            n_, err = buf.w.Write(p[:n_])
            p = p[n_:]
            n += n_
	}
	return n, err
}

func Copy(w io.Writer, r io.Reader, buflen int) (int64, error) {
        var pipe BufferedPipe
        buf := limitedBuf{
            r: &pipe,
            w: &pipe,
        }
        buf.s.Add(buflen)
	go func() {
		io.Copy(&buf, r)
		pipe.Close()
	}()
	return io.Copy(w, &buf)
}
