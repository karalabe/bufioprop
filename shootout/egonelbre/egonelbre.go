package egonelbre

import (
	"io"
	"runtime"
	"sync/atomic"
)

type process chan struct{}

func newprocess() process { return make(process) }
func (p process) exit()   { close(p) }
func (p process) wait()   { <-p }
func (p process) exited() bool {
	select {
	case <-p:
		return true
	default:
	}
	return false
}

func Copy(dst io.Writer, src io.Reader, buffer int) (written int64, err error) {
	buf := make([]byte, buffer)
	buflen := int32(len(buf))

	// data[  low : high ] is the written part of buf
	// data[ high : low  ] is the unwritten part of the buf
	low, high := int32(0), int32(0)

	var rerr, werr error

	r := newprocess()
	w := newprocess()

	go func() {
		defer r.exit()

		h := atomic.LoadInt32(&high)
		for rerr == nil && !w.exited() {
			l := atomic.LoadInt32(&low)

			// are we full, go to sleep
			for (h+1)%buflen == l {
				if w.exited() {
					return
				}
				runtime.Gosched()
				l = atomic.LoadInt32(&low)
			}

			var next []byte
			if l <= h {
				next = buf[h:]
			} else if h < l {
				next = buf[h:l]
			}

			var nr int
			for len(next) > 0 && rerr == nil && !w.exited() {
				nr, rerr = src.Read(next)
				next = next[nr:]
				h = (h + int32(nr)) % buflen
				atomic.StoreInt32(&high, h)
			}
		}
	}()

	go func() {
		defer w.exit()

		l := atomic.LoadInt32(&low)
		for werr == nil {
			h := atomic.LoadInt32(&high)

			// are we empty, go to sleep or exit
			for l == h {
				if r.exited() {
					return
				}
				runtime.Gosched()
				h = atomic.LoadInt32(&high)
			}

			var next []byte
			if l < h {
				next = buf[l:h]
			} else if h <= l {
				next = buf[l:]
			}

			var nr int
			for len(next) > 0 && werr == nil {
				nr, werr = dst.Write(next)
				atomic.AddInt64(&written, int64(nr))
				l = (l + int32(nr)) % buflen
				atomic.StoreInt32(&low, l)
				next = next[nr:]
			}
		}
	}()

	// Wait until both finish and return
	w.wait()
	r.wait()
	return
}
