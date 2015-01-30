// Package bufioprop contains extension functions to the bufio package.
package bufioprop

import "io"

// Copy copies from src to dst until either EOF is reached on src or an error
// occurs. It returns the number of bytes copied and the first error encountered
// while copying, if any.
//
// A successful Copy returns err == nil, not err == EOF. Because Copy is defined
// to read from src until EOF, it does not treat an EOF from Read as an error to
// be reported.
//
// Internally, one goroutine is reading the src, moving the data into an internal
// buffer, and another moving from the buffer to the writer. This permits both
// endpoints to run simultaneously, without one blocking the other.
func Copy(dst io.Writer, src io.Reader, buffer int) (written int64, err error) {
	pr, pw := Pipe(buffer)

	// Run one copy to push data into the buffered pipe
	errc := make(chan error)
	go func() {
		_, err := io.Copy(pw, src)
		pw.Close()
		errc <- err
	}()
	// Run another copy to stream data out into the sink
	written, errOut := io.Copy(dst, pr)

	errIn := <-errc
	if errOut != nil {
		return written, errOut
	}
	return written, errIn
}
