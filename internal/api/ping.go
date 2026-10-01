package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/uptimy/agent/internal/scheduler"
)

// A heartbeat's job reports in at /ping/<token>, the same conventions as
// healthchecks.io:
//
//	/ping/<token>          the job finished
//	/ping/<token>/start    the job started (its finish then gives the duration)
//	/ping/<token>/fail     the job finished and failed
//	/ping/<token>/<code>   the job exited with code; 0 is success
//
// GET, POST and HEAD all work. A POST body (e.g. the tail of a log) is kept
// as the run's message. The token is the only credential, so these are
// public, and they answer in plain text for curl.

const maxPingMessage = 500

func (s *Server) pingRoutes(mux *http.ServeMux) {
	for _, method := range []string{http.MethodGet, http.MethodPost} { // GET also matches HEAD
		mux.HandleFunc(method+" /ping/{token}", s.ping(scheduler.SignalSuccess))
		mux.HandleFunc(method+" /ping/{token}/start", s.ping(scheduler.SignalStart))
		mux.HandleFunc(method+" /ping/{token}/fail", s.ping(scheduler.SignalFailure))
		mux.HandleFunc(method+" /ping/{token}/{code}", s.pingExitCode)
	}
}

func (s *Server) ping(signal scheduler.Signal) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.deliverPing(w, r, signal, pingBody(r))
	}
}

func (s *Server) pingExitCode(w http.ResponseWriter, r *http.Request) {
	code, err := strconv.Atoi(r.PathValue("code"))
	if err != nil || code < 0 || code > 255 {
		http.Error(w, "use /ping/<token>, /start, /fail or an exit code from 0 to 255", http.StatusNotFound)
		return
	}
	if code == 0 {
		s.deliverPing(w, r, scheduler.SignalSuccess, pingBody(r))
		return
	}
	msg := "exit code " + strconv.Itoa(code)
	if body := pingBody(r); body != "" {
		msg += ": " + body
	}
	s.deliverPing(w, r, scheduler.SignalFailure, msg)
}

func (s *Server) deliverPing(w http.ResponseWriter, r *http.Request, signal scheduler.Signal, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.Scheduler.Ping(r.PathValue("token"), signal, message); err != nil {
		if errors.Is(err, scheduler.ErrUnknownToken) {
			http.Error(w, "unknown ping URL; copy it from the heartbeat's page", http.StatusNotFound)
			return
		}
		s.Log.Error("ping failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	_, _ = io.WriteString(w, "OK\n")
}

// pingBody returns the start of a POST body, as the run's message.
func pingBody(r *http.Request) string {
	if r.Method != http.MethodPost {
		return ""
	}
	b, _ := io.ReadAll(io.LimitReader(r.Body, 4*maxPingMessage))
	msg := strings.TrimSpace(strings.ToValidUTF8(string(b), "�"))
	if utf8.RuneCountInString(msg) > maxPingMessage {
		msg = string([]rune(msg)[:maxPingMessage]) + "…"
	}
	return msg
}
