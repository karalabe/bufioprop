package egonelbre

import (
	"io"
	"runtime"
	"sync/atomic"
)

type process struct {
	quit  chan struct{}
	sleep chan struct{}
}

func newprocess() process {
	return process{
		quit:  make(chan struct{}),
		sleep: make(chan struct{}, 1),
	}
}
func (p process) exit() { close(p.quit) }
func (p process) wait() { <-p.quit }
func (p process) exited() bool {
	select {
	case <-p.quit:
		return true
	default:
		return false
	}
}

func (p process) waitchange(other process, expect int32, pv *int32) (exited bool) {
	// say we are sleep
	p.sleep <- struct{}{}
	v := atomic.LoadInt32(pv)

	const maxSpin = 16
	for i := 0; i < maxSpin && expect == v; i += 1 {
		runtime.Gosched()
		v = atomic.LoadInt32(pv)
	}

	// go to sleep
	for expect == v {
		select {
		case <-other.quit:
			return true
		case p.sleep <- struct{}{}:
		}
		v = atomic.LoadInt32(pv)
	}

	p.unwait()
	return false
}

func (p process) unwait() {
	// clear sleeping
	select {
	case <-p.sleep:
	default:
	}
}

func maxsize(a int) int {
	const maxchunk = 16 << 10
	if a > maxchunk {
		return maxchunk
	}
	return a
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

		var next []byte

		h := atomic.LoadInt32(&high)
		for rerr == nil && !w.exited() {
			l := atomic.LoadInt32(&low)

			// are we full
			if (h+1)%buflen == l {
				if r.waitchange(w, l, &low) {
					return
				}
				l = atomic.LoadInt32(&low)
			}

			switch {
			case l == 0:
				next = buf[h : len(buf)-1]
			case h < l:
				next = buf[h : l-1]
			case l <= h:
				next = buf[h:]
			}

			var nr int
			for len(next) > 0 && rerr == nil && !w.exited() {
				nr, rerr = src.Read(next[:maxsize(len(next))])
				next = next[nr:]
				h = (h + int32(nr)) % buflen
				atomic.StoreInt32(&high, h)
				w.unwait()
			}
		}
	}()

	go func() {
		defer w.exit()

		var next []byte
		l := atomic.LoadInt32(&low)
		for werr == nil {
			h := atomic.LoadInt32(&high)

			if l == h {
				exited := w.waitchange(r, h, &high)
				h = atomic.LoadInt32(&high)
				if l == h && exited {
					return
				}
			}

			if l < h {
				next = buf[l:h]
			} else if h <= l {
				next = buf[l:]
			}

			var nr int
			for len(next) > 0 && werr == nil {
				nr, werr = dst.Write(next[:maxsize(len(next))])
				written += int64(nr)
				l = (l + int32(nr)) % buflen
				atomic.StoreInt32(&low, l)
				r.unwait()
				next = next[nr:]
			}
		}
	}()

	// Wait until both finish and return
	w.wait()
	r.wait()

	switch {
	case rerr == nil || rerr == io.EOF:
		return written, werr
	// what if both errors happened?
	default:
		return written, rerr
	}
}
