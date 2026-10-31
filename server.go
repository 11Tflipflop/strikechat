package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

type Server struct {
	room       string
	key        string // empty = no key needed
	hub        *Hub
	sendLim    *limiter // limits sending messages
	authLim    *limiter // limits wrong key guesses
	maxClients int32
	clients    int32
	page       []byte
	done       chan struct{} // closed when the server is shutting down
}

func NewServer(room, key string, hub *Hub, page []byte, maxClients int) *Server {
	return &Server{
		room:       room,
		key:        key,
		hub:        hub,
		sendLim:    newLimiter(1, 5),          // 1 message/sec, bursts of 5
		authLim:    newLimiter(10.0/60.0, 10), // ~10 wrong keys per minute
		maxClients: int32(maxClients),
		page:       page,
		done:       make(chan struct{}),
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /api/info", s.auth(s.handleInfo))
	mux.HandleFunc("GET /api/stream", s.auth(s.handleStream))
	mux.HandleFunc("POST /api/send", s.auth(s.handleSend))
	return mux
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// checks the ?k= key on every api call
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		k := r.URL.Query().Get("k")
		if subtle.ConstantTimeCompare([]byte(k), []byte(s.key)) != 1 {
			if !s.authLim.Allow(clientIP(r)) {
				http.Error(w, "too many tries", http.StatusTooManyRequests)
				return
			}
			http.Error(w, "bad link", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy",
		"default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'")
	w.Write(s.page)
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"room":   s.room,
		"online": s.hub.Online(),
	})
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if !s.sendLim.Allow(clientIP(r)) {
		http.Error(w, "slow down", http.StatusTooManyRequests)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var in struct {
		Name string `json:"name"`
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	text, ok := cleanText(in.Text)
	if !ok {
		http.Error(w, "message empty or too long", http.StatusBadRequest)
		return
	}

	s.hub.Add(cleanName(in.Name), text)
	w.WriteHeader(http.StatusNoContent)
}

// Server-Sent Events: the browser opens this once and we keep pushing messages.
// Browsers reconnect on their own and send Last-Event-ID, so nothing is missed.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	if atomic.AddInt32(&s.clients, 1) > s.maxClients {
		atomic.AddInt32(&s.clients, -1)
		http.Error(w, "room is full", http.StatusServiceUnavailable)
		return
	}
	defer atomic.AddInt32(&s.clients, -1)

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")

	var last int64
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		last, _ = strconv.ParseInt(v, 10, 64)
	}

	backlog, ch := s.hub.Subscribe(last)
	defer s.hub.Unsubscribe(ch)

	for _, m := range backlog {
		writeEvent(w, m)
	}
	fl.Flush()

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.done:
			return
		case m, ok := <-ch:
			if !ok {
				return // we were cut off for being slow, browser will reconnect
			}
			writeEvent(w, m)
			fl.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n") // keeps the connection from timing out
			fl.Flush()
		}
	}
}

func writeEvent(w io.Writer, m Message) {
	b, _ := json.Marshal(m)
	fmt.Fprintf(w, "id: %d\ndata: %s\n\n", m.ID, b)
}
