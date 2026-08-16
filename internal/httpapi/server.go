package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hn-tran/n0ding-lab/internal/bundle"
	"github.com/hn-tran/n0ding-lab/internal/core"
	webassets "github.com/hn-tran/n0ding-lab/web"
)

type Server struct {
	Mode  string
	Store *core.Store
	Token string
}

const maxBodyBytes int64 = 1 << 20
const maxSSEBacklog = 1000

func New(mode string, store *core.Store) http.Handler {
	return NewAuthenticated(mode, store, "")
}

func NewAuthenticated(mode string, store *core.Store, token string) http.Handler {
	s := &Server{Mode: mode, Store: store, Token: token}
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", s.health)
	m.HandleFunc("GET /api/v1/runs", s.listRuns)
	m.HandleFunc("POST /api/v1/runs", s.createRun)
	m.HandleFunc("GET /api/v1/runs/{id}/events", s.events)
	m.HandleFunc("POST /api/v1/runs/{id}/events", s.appendEvent)
	m.HandleFunc("GET /api/v1/runs/{id}/projection", s.projection)
	m.HandleFunc("POST /api/v1/fixtures", s.fixture)
	m.HandleFunc("GET /api/v1/runs/{id}/export", s.exportRun)
	m.HandleFunc("POST /api/v1/replay/import", s.importReplay)
	m.Handle("GET /", http.FileServer(http.FS(webassets.FS)))
	return s.security(m)
}

func (s *Server) exportRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.owns(id) {
		write(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	raw, err := bundle.Export(s.Store, id)
	if err != nil {
		write(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="n0ding-replay.json"`)
	w.Write(raw)
}
func (s *Server) importReplay(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, bundle.MaxBundleBytes)
	defer r.Body.Close()
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		write(w, http.StatusBadRequest, map[string]string{"error": "invalid or oversized replay bundle"})
		return
	}
	p, err := bundle.VerifyAndReplay(raw)
	if err != nil {
		write(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	write(w, http.StatusOK, map[string]any{"mode": "replay", "projection": p})
}

func (s *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		if s.Token != "" && strings.HasPrefix(r.URL.Path, "/api/") {
			provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(provided), []byte(s.Token)) != 1 {
				write(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) owns(id string) bool { run, ok := s.Store.GetRun(id); return ok && run.Mode == s.Mode }

func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, map[string]any{"ok": true, "mode": s.Mode})
}

func (s *Server) listRuns(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, map[string]any{"runs": s.Store.ListRuns(s.Mode)})
}

func (s *Server) createRun(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var x struct{ ID, Name string }
	if json.NewDecoder(r.Body).Decode(&x) != nil || x.ID == "" {
		write(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}
	run := core.Run{ID: x.ID, Name: x.Name, Mode: s.Mode}
	if err := s.Store.CreateRun(run); err != nil {
		write(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	run, _ = s.Store.GetRun(x.ID)
	write(w, http.StatusCreated, run)
}

func (s *Server) appendEvent(w http.ResponseWriter, r *http.Request) {
	if !s.owns(r.PathValue("id")) {
		write(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var x struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if json.NewDecoder(r.Body).Decode(&x) != nil || x.Type == "" {
		write(w, http.StatusBadRequest, map[string]string{"error": "type required"})
		return
	}
	e, err := s.Store.Append(r.PathValue("id"), x.Type, x.Data)
	if err != nil {
		write(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	write(w, http.StatusCreated, e)
}

func afterID(r *http.Request) (int64, error) {
	raw := r.URL.Query().Get("after")
	if raw == "" {
		raw = r.Header.Get("Last-Event-ID")
	}
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid event cursor")
	}
	return n, nil
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.owns(id) {
		write(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	cursor, err := afterID(r)
	if err != nil {
		write(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		events, more := s.Store.EventsLimit(id, cursor, maxSSEBacklog)
		if more {
			write(w, http.StatusConflict, map[string]string{"error": "event backlog exceeds bounded response; advance cursor"})
			return
		}
		write(w, http.StatusOK, map[string]any{"events": events})
		return
	}
	if _, more := s.Store.EventsLimit(id, cursor, maxSSEBacklog); more {
		write(w, http.StatusConflict, map[string]string{"error": "event backlog exceeds stream limit; resync from snapshot"})
		return
	}
	f, ok := w.(http.Flusher)
	if !ok {
		write(w, http.StatusInternalServerError, map[string]string{"error": "stream unsupported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	send := func() bool {
		events, more := s.Store.EventsLimit(id, cursor, maxSSEBacklog)
		if more {
			return false
		}
		for _, e := range events {
			b, _ := json.Marshal(e)
			if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.ID, e.Type, b); err != nil {
				return false
			}
			cursor = e.ID
		}
		f.Flush()
		return true
	}
	for {
		ch, _ := s.Store.Subscribe()
		if !send() {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			continue
		case <-time.After(15 * time.Second):
			fmt.Fprint(w, ": keepalive\n\n")
			f.Flush()
		}
	}
}

func (s *Server) projection(w http.ResponseWriter, r *http.Request) {
	if !s.owns(r.PathValue("id")) {
		write(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	upto, _ := strconv.ParseInt(r.URL.Query().Get("upto"), 10, 64)
	p, err := s.Store.Replay(r.PathValue("id"), upto)
	if err != nil {
		write(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	write(w, http.StatusOK, p)
}

func (s *Server) fixture(w http.ResponseWriter, _ *http.Request) {
	run, err := core.LoadFixture(s.Store, s.Mode)
	if err != nil {
		write(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	write(w, http.StatusCreated, run)
}
