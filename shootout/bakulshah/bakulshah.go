package bakulshah

import "io"

// Shootout mods: raised the size to API level, counted and returned the written count.

func Copy(w io.Writer, r io.Reader, size int) (int64, error) {
	written := int64(0)

	done := make(chan bool)
	size1 := size / 16
	buf := make([]byte, size)
	hasData := make(chan int, 16)
	hasSpace := make(chan int, 16)
	var errorstop error

	writeout := func() {
		wbase := 0
		for {
			count := <-hasData
			if count == -1 {
				done <- true
				return
			}
			//fmt.Printf("writer: [%x:%x)\n", wbase, wbase+count)
			next := wbase + size1
			if next >= size {
				next = 0
			}
			for {
				n, err := w.Write(buf[wbase : wbase+count])
				if err != nil {
					errorstop = err
					done <- true
					return
				}
				written += int64(n)
				if n == count {
					break
				}
				wbase += n
				count -= n
			}
			hasSpace <- next
			wbase = next
		}
	}
	readin := func() error {
		rbase := 0
		tokens := 16
		for {
			n, err := r.Read(buf[rbase : rbase+size1])
			//fmt.Printf("reader: [%x:%x)\n", rbase, rbase+n)
			if n > 0 {
				hasData <- n
			}
			if err != nil || n == 0 {
				hasData <- -1
				<-done
				return err
			}
			rbase += size1
			if rbase >= size {
				rbase = 0
			}
			if tokens == 0 {
				<-hasSpace
			} else {
				tokens--
			}
			if errorstop != nil {
				return errorstop
			}
		}
	}

	go writeout()
	return written, readin()
}
