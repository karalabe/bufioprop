package mattharden

import (
	"sync"
)

type sema struct {
	mu sync.Mutex
	c  sync.Cond
	s  int
}

func (s *sema) Init(size int) {
	s.c.L = &s.mu
	s.s = size
}

// Add pushes the semaphore up by n
func (s *sema) Add(n int) {
	s.mu.Lock()
	if s.s == 0 {
		s.s += n
		s.c.Broadcast()
	} else {
		s.s += n
	}
	s.mu.Unlock()
	return
}

// Sub pulls the semaphore down as much as possible, less than or equal to n.
// It waits until it can return some number greater than zero.
func (s *sema) Sub(n int) int {
	s.mu.Lock()
	if s.c.L == nil {
		s.c.L = &s.mu
	}
	for s.s == 0 {
		s.c.Wait()
	}
	if n > s.s {
		n = s.s
	}
	s.s -= n
	s.mu.Unlock()
	return n
}
