package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gabesullice/recui/pkg/config"
	"github.com/gabesullice/recui/pkg/recfile"
)

// Config holds the server configuration.
type Config struct {
	Addr        string // e.g. "127.0.0.1"
	Port        int    // e.g. 8080
	RecfilePath string
	WebDir      string          // if non-empty, serve assets from disk instead of embedded FS
	UIConfig    config.UIConfig // display configuration loaded from --config; nil means defaults
}

// srv holds the runtime state shared across handlers.
type srv struct {
	cfg           Config
	types         atomic.Pointer[[]recfile.RecordType]
	lastMtime     atomic.Int64
	lastSize      atomic.Int64
	keylessWarned sync.Map // set of type names already warned about keyless access
}

// Run initialises and starts the HTTP server. It blocks until ctx is cancelled
// or the server exits with an error.
//
// The onReady callback, if non-nil, is invoked exactly once after the recfile
// has been parsed and before ListenAndServe is called — giving the caller
// (e.g. cmd/recui) access to the parsed types without re-parsing the file.
//
// When ctx is cancelled Run triggers http.Server.Shutdown with a bounded
// timeout and returns nil on clean drain; other errors are propagated.
func Run(ctx context.Context, cfg Config, onReady func(types []recfile.RecordType)) error {
	types, err := recfile.ParseFile(cfg.RecfilePath)
	if err != nil {
		return fmt.Errorf("parsing recfile: %w", err)
	}
	if onReady != nil {
		onReady(types)
	}

	s := &srv{cfg: cfg}
	s.types.Store(&types)

	// Seed mtime/size from the initial stat.
	if info, statErr := os.Stat(cfg.RecfilePath); statErr == nil {
		s.lastMtime.Store(info.ModTime().UnixNano())
		s.lastSize.Store(info.Size())
	}

	// Background poller: re-parse on mtime change.
	go s.pollLoop()

	addr := fmt.Sprintf("%s:%d", cfg.Addr, cfg.Port)
	slog.Info("listening", "addr", addr, "recfile", cfg.RecfilePath)

	hs := &http.Server{
		Addr:    addr,
		Handler: requestLogger(s.buildMux()),
	}

	// Run ListenAndServe in a goroutine so we can listen for ctx cancellation
	// and trigger a graceful Shutdown. ListenAndServe returns ErrServerClosed
	// after a successful Shutdown — that is not an error from our caller's
	// perspective.
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- hs.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := hs.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		// Drain the serve goroutine's final return (should be ErrServerClosed).
		<-serveErr
		return nil
	}
}

// buildMux constructs and returns the HTTP handler mux for the server.
func (s *srv) buildMux() http.Handler {
	mux := http.NewServeMux()
	mux.Handle(patternHome, methods(map[string]handlerFunc{
		"GET":  s.handleHome,
		"HEAD": s.handleHead,
	}))
	mux.Handle(patternTypeList, methods(map[string]handlerFunc{
		"GET": s.handleType,
	}))
	mux.Handle(patternRecord, methods(map[string]handlerFunc{
		"GET": s.handleRecord,
	}))

	// Static asset handler: serve from disk if WebDir is set (development),
	// otherwise serve from the embedded web/ FS.
	var staticFS http.FileSystem
	if s.cfg.WebDir != "" {
		staticFS = http.Dir(s.cfg.WebDir)
	} else {
		staticFS = http.FS(webFS)
	}
	mux.Handle("/style.css", http.FileServer(staticFS))
	// /vendor/ serves the DM Mono font referenced by style.css.
	mux.Handle("/vendor/", http.FileServer(staticFS))
	return mux
}

// pollLoop checks the recfile for changes every 2 seconds and re-parses on mtime/size change.
func (s *srv) pollLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		info, err := os.Stat(s.cfg.RecfilePath)
		if err != nil {
			slog.Warn("stat recfile failed", "err", err)
			continue
		}
		mtime := info.ModTime().UnixNano()
		size := info.Size()
		if mtime == s.lastMtime.Load() && size == s.lastSize.Load() {
			continue
		}
		types, err := recfile.ParseFile(s.cfg.RecfilePath)
		if err != nil {
			slog.Warn("re-parse recfile failed; retaining previous state", "err", err)
			continue
		}
		s.types.Store(&types)
		s.lastMtime.Store(mtime)
		s.lastSize.Store(size)
		slog.Info("recfile reloaded", "recfile", s.cfg.RecfilePath)
	}
}

// etag computes a quoted ETag from the current mtime and size.
func (s *srv) etag() string {
	mtime := s.lastMtime.Load()
	size := s.lastSize.Load()
	return fmt.Sprintf("%q", fmt.Sprintf("%x", mtime^size))
}

// methods returns an http.Handler that dispatches to the handler registered
// for the request method. Unmatched methods return 405 with an Allow header
// listing the registered methods, per RFC 7231 §6.5.5 (MUST). The Allow list
// is sorted for deterministic output.
func methods(handlers map[string]handlerFunc) http.Handler {
	allowed := make([]string, 0, len(handlers))
	for m := range handlers {
		allowed = append(allowed, m)
	}
	sort.Strings(allowed)
	allow := strings.Join(allowed, ", ")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f, ok := handlers[r.Method]; ok {
			if err := f(w, r); err != nil {
				writeError(w, r, err)
			}
			return
		}
		w.Header().Set("Allow", allow)
		writeError(w, r, errMethodNotAllowed)
	})
}
