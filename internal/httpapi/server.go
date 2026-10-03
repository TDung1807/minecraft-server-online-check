package httpapi

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"minecraft-monitor/internal/monitor"
)

//go:embed web/*
var web embed.FS

var processStarted = time.Now()

type visitor struct {
	start    time.Time
	requests int
	streams  int
	seen     time.Time
}
type API struct {
	Store      *monitor.Store
	Origin     string
	TrustProxy bool
	mu         sync.Mutex
	visitors   map[string]*visitor
	streams    int
}

func New(s *monitor.Store, origin string, trust bool) *API {
	return &API{Store: s, Origin: origin, TrustProxy: trust, visitors: make(map[string]*visitor)}
}
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/status", a.status)
	mux.HandleFunc("GET /api/v1/events", a.events)
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !a.Store.Ready.Load() {
			http.Error(w, "not ready", 503)
			return
		}
		w.Write([]byte("ready\n"))
	})
	assets, _ := fs.Sub(web, "web")
	mux.Handle("GET /", http.FileServer(http.FS(assets)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			origin := r.Header.Get("Origin")
			if origin != "" {
				expected := a.Origin
				if expected == "" {
					scheme := "http"
					if r.TLS != nil {
						scheme = "https"
					}
					if a.trusted(r) && r.Header.Get("X-Forwarded-Proto") == "https" {
						scheme = "https"
					}
					expected = scheme + "://" + r.Host
				}
				if origin != expected {
					http.Error(w, "origin not allowed", 403)
					return
				}
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
				w.WriteHeader(204)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func (a *API) trusted(r *http.Request) bool {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	return a.TrustProxy && ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}
func (a *API) ip(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if a.trusted(r) { // Caddy overwrites this header; never trust arbitrary public peers.
		forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0])
		if ip := net.ParseIP(forwarded); ip != nil {
			return ip.String()
		}
	}
	return host
}
func (a *API) allow(ip string, stream bool) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	if len(a.visitors) > 1024 {
		for key, v := range a.visitors {
			if v.streams == 0 && now.Sub(v.seen) > time.Minute {
				delete(a.visitors, key)
			}
		}
	}
	v := a.visitors[ip]
	if v == nil {
		if len(a.visitors) >= 10000 {
			return false
		}
		v = &visitor{start: now}
		a.visitors[ip] = v
	}
	v.seen = now
	if stream {
		if a.streams >= 500 || v.streams >= 5 {
			return false
		}
		v.streams++
		a.streams++
		return true
	}
	if now.Sub(v.start) >= time.Minute {
		v.start = now
		v.requests = 0
	}
	if v.requests >= 60 {
		return false
	}
	v.requests++
	return true
}
func limited(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "60")
	http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
}
func (a *API) status(w http.ResponseWriter, r *http.Request) {
	if !a.allow(a.ip(r), false) {
		limited(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(a.Store.Snapshot())
}
func (a *API) events(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		return
	}
	ip := a.ip(r)
	if !a.allow(ip, true) {
		limited(w)
		return
	}
	defer func() { a.mu.Lock(); a.streams--; a.visitors[ip].streams--; a.mu.Unlock() }()
	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	ch, cancel := a.Store.Subscribe()
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	write := func(value string) error {
		if err := controller.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
			return err
		}
		if _, err := fmt.Fprint(w, value); err != nil {
			return err
		}
		return controller.Flush()
	}
	if err := write("retry: 3000\n\n"); err != nil {
		return
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case snap := <-ch:
			data, err := json.Marshal(snap)
			if err != nil {
				return
			}
			if err = write(fmt.Sprintf("id: %d\nevent: status\ndata: %s\n\n", snap.Sequence, data)); err != nil {
				return
			}
		case <-heartbeat.C:
			if err := write(": heartbeat\n\n"); err != nil {
				return
			}
		}
	}
}
func (a *API) Metrics() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		s := a.Store.Snapshot()
		age := -1.0
		if s.LastSuccessAt != nil {
			age = time.Since(*s.LastSuccessAt).Seconds()
		}
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprintf(w, "process_start_time_seconds %g\nmc_data_age_seconds %g\nmc_consecutive_failures %d\nmc_query_attempts_total %d\nmc_query_failures_total %d\nmc_sse_subscribers %d\ngo_goroutines %d\ngo_heap_alloc_bytes %d\ngo_heap_sys_bytes %d\n", float64(processStarted.Unix()), age, s.ConsecutiveFailures, a.Store.Attempts.Load(), a.Store.Failures.Load(), a.Store.Subscribers(), runtime.NumGoroutine(), mem.HeapAlloc, mem.HeapSys)
	})
	return mux
}
