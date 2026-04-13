package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// handlerFunc is an http.Handler whose ServeHTTP delegates to a function that
// returns an error. Error responses are written by writeError.
type handlerFunc func(http.ResponseWriter, *http.Request) error

func (f handlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := f(w, r); err != nil {
		writeError(w, r, err)
	}
}

// httpError is a typed error carrying an HTTP status code.
type httpError struct {
	status int
	title  string
}

func (e *httpError) Error() string { return e.title }

var (
	errNotFound         = &httpError{status: http.StatusNotFound, title: "Not Found"}
	errMethodNotAllowed = &httpError{status: http.StatusMethodNotAllowed, title: "Method Not Allowed"}
	errNotAcceptable    = &httpError{status: http.StatusNotAcceptable, title: "Not Acceptable"}
)

// problemDetail is the RFC 9457 response body.
type problemDetail struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

// writeError writes an RFC 9457 Problem Details response. It must not include
// file paths, field values, or stack traces in the body.
func writeError(w http.ResponseWriter, _ *http.Request, err error) {
	var he *httpError
	if errors.As(err, &he) {
		writeProblem(w, he.status, he.title, he.title)
		return
	}
	// Unexpected error: log before writing so the log entry precedes the response.
	slog.Error("internal server error", "err", err)
	writeProblem(w, http.StatusInternalServerError, "Internal Server Error", "An unexpected error occurred.")
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problemDetail{
		Type:   "about:blank",
		Title:  title,
		Status: status,
		Detail: detail,
	})
}

// requestLogger wraps an http.Handler and logs each completed request.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// statusRecorder captures the HTTP status code written by a handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
