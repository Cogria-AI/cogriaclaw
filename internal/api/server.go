// Package api exposes a small HTTP control surface so external systems can
// push messages or trigger skills on demand. Three endpoints:
//
//	GET  /healthz   liveness + WhatsApp connection status (no auth)
//	POST /send      send a message directly, bypassing the LLM (bearer auth)
//	POST /trigger   run a named skill, optionally announce the result (bearer auth)
//
// Bind to 127.0.0.1 and put any public exposure behind your own tunnel/proxy.
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/Cogria-AI/cogriaclaw/internal/tool"
	"github.com/Cogria-AI/cogriaclaw/internal/wa"
)

type Deps struct {
	WA      *wa.Client
	Tools   func() *tool.Registry // current tool registry (a getter so it survives config reload)
	Token   string                // bearer token; required for /send and /trigger
	Started time.Time
}

type Server struct {
	deps Deps
	srv  *http.Server
}

func New(listen string, deps Deps) *Server {
	s := &Server{deps: deps}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.Handle("POST /send", s.auth(http.HandlerFunc(s.handleSend)))
	mux.Handle("POST /trigger", s.auth(http.HandlerFunc(s.handleTrigger)))
	s.srv = &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

// ListenAndServe blocks serving requests. Returns http.ErrServerClosed on
// graceful shutdown.
func (s *Server) ListenAndServe() error {
	return s.srv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
