package api

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/uptimy/agent/internal/statuspage"
)

// The status page's own domain (statuspage.Settings.Domain), e.g.
// status.example.com, serves only the status page: the page itself at "/",
// what it loads, and heartbeat pings, so jobs outside the network can reach
// /ping on the one public address. Sign-in, the API and /metrics aren't
// there, so the dashboard can stay on a private address.

// statusViewMeta tells the web UI to show only the status page.
const statusViewMeta = `<meta name="uptimy-view" content="status">`

func (s *Server) setStatusDomain(domain string) { s.statusDomain.Store(&domain) }

func (s *Server) isStatusDomain(r *http.Request) bool {
	d := s.statusDomain.Load()
	return d != nil && *d != "" && statuspage.NormalizeHost(r.Host) == *d
}

// statusDomainOnly limits requests to the status page's domain to what the
// page needs; other hosts get next unchanged.
func (s *Server) statusDomainOnly(next, ui http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.isStatusDomain(r) {
			next.ServeHTTP(w, r)
			return
		}
		p := r.URL.Path
		switch {
		case p == "/" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
			serveStatusView(w, r, ui)
		case p == "/status":
			http.Redirect(w, r, "/", http.StatusMovedPermanently)
		case p == "/healthz", p == "/api/status", strings.HasPrefix(p, "/api/status/logo/"),
			strings.HasPrefix(p, "/ping/"),
			strings.HasPrefix(p, "/assets/"), strings.HasPrefix(p, "/brand/"), p == "/favicon.svg":
			next.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

// serveStatusView serves the UI's index.html with statusViewMeta added, so
// the app renders the status page at "/".
func serveStatusView(w http.ResponseWriter, r *http.Request, ui http.Handler) {
	r = r.Clone(r.Context())
	r.Header.Del("Accept-Encoding") // the body is edited below
	buf := &bufferedResponse{header: http.Header{}, code: http.StatusOK}
	ui.ServeHTTP(buf, r)

	body := buf.body.Bytes()
	if buf.code == http.StatusOK {
		body = bytes.Replace(body, []byte("<head>"), []byte("<head>"+statusViewMeta), 1)
	}
	for k, v := range buf.header {
		w.Header()[k] = v
	}
	w.Header().Del("Content-Length")
	w.WriteHeader(buf.code)
	_, _ = w.Write(body)
}

type bufferedResponse struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header         { return b.header }
func (b *bufferedResponse) WriteHeader(code int)        { b.code = code }
func (b *bufferedResponse) Write(p []byte) (int, error) { return b.body.Write(p) }
