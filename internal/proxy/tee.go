package proxy

import (
	"io"
	"net/http"
)

type teeReadCloser struct {
	io.Reader
	io.Closer
}

func newTeeReadCloser(rc io.ReadCloser, w io.Writer) io.ReadCloser {
	return &teeReadCloser{
		io.TeeReader(rc, w),
		rc,
	}
}

// Response Body Tee
type teeResponseWriter struct {
	http.ResponseWriter
	writer io.Writer
}

func (w *teeResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
