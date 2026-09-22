package ast

import "io"

// printWriter holds the writer that a Print* function writes to. It keeps the
// first write error in err, and it does not write again after an error.
type printWriter struct {
	w   io.Writer
	err error
}

// Write is the io.Writer method. If err has an error, Write does not write the
// bytes and gives that error again.
func (p *printWriter) Write(b []byte) (int, error) {
	if p.err != nil {
		return 0, p.err
	}
	var n int
	n, p.err = p.w.Write(b)
	return n, p.err
}

// emit writes the bytes. It does not give an error, because err keeps the error.
func (p *printWriter) emit(b []byte) {
	_, _ = p.Write(b)
}
