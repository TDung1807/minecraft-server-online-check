package httpapi

import (
	"bufio"
	"fmt"
	"minecraft-monitor/internal/minecraft"
	"minecraft-monitor/internal/monitor"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestREST(t *testing.T) {
	s := monitor.New("test")
	a := New(s, "", false)
	h := a.Handler()
	for i := 0; i < 61; i++ {
		r := httptest.NewRequest("GET", "/api/v1/status", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		want := 200
		if i == 60 {
			want = 429
		}
		if w.Code != want {
			t.Fatal(w.Code)
		}
		if i == 0 && (!strings.Contains(w.Body.String(), `"playersOnline":null`) || w.Header().Get("Cache-Control") != "no-store") {
			t.Fatal(w.Body.String())
		}
	}
	if s.Attempts.Load() != 0 {
		t.Fatal("REST queried Minecraft")
	}
}
func TestHealthAndOrigins(t *testing.T) {
	s := monitor.New("test")
	h := New(s, "https://example.com", false).Handler()
	for _, tc := range []struct {
		path, origin string
		code         int
	}{{"/health/live", "", 200}, {"/health/ready", "", 503}, {"/api/v1/status", "https://evil.com", 403}, {"/api/v1/status", "https://example.com", 200}} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Fatal(tc, w.Code)
		}
	}
	s.Ready.Store(true)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/health/ready", nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
}
func TestSSE(t *testing.T) {
	s := monitor.New("test")
	a := New(s, "", false)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	client := &http.Client{Timeout: 2 * time.Second}
	var responses []*http.Response
	defer func() {
		for _, r := range responses {
			r.Body.Close()
		}
	}()
	for i := 0; i < 5; i++ {
		r, e := client.Get(srv.URL + "/api/v1/events")
		if e != nil {
			t.Fatal(e)
		}
		responses = append(responses, r)
		if r.StatusCode != 200 {
			t.Fatal(r.StatusCode)
		}
	}
	blocked, e := client.Get(srv.URL + "/api/v1/events")
	if e != nil {
		t.Fatal(e)
	}
	blocked.Body.Close()
	if blocked.StatusCode != 429 {
		t.Fatal(blocked.StatusCode)
	}
	scanner := bufio.NewScanner(responses[0].Body)
	readData := func() string {
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "data: ") {
				return scanner.Text()
			}
		}
		t.Fatal("missing event", scanner.Err())
		return ""
	}
	if data := readData(); !strings.Contains(data, `"state":"unknown"`) {
		t.Fatal(data)
	}
	now := time.Now()
	s.Record(minecraft.Result{Online: 4, Max: 20}, nil, now, now)
	if data := readData(); !strings.Contains(data, `"playersOnline":4`) {
		t.Fatal(data)
	}
	for _, r := range responses {
		r.Body.Close()
	}
	deadline := time.Now().Add(time.Second)
	for s.Subscribers() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if s.Subscribers() != 0 {
		t.Fatal("subscriber leak")
	}
	fresh, e := client.Get(srv.URL + "/api/v1/events")
	if e != nil {
		t.Fatal(e)
	}
	defer fresh.Body.Close()
	scanner = bufio.NewScanner(fresh.Body)
	if data := readData(); !strings.Contains(data, `"playersOnline":4`) {
		t.Fatal(data)
	}
}
func TestLimitsAndProxy(t *testing.T) {
	a := New(monitor.New("test"), "", true)
	for i := 0; i < 500; i++ {
		if !a.allow(fmt.Sprint(i), true) {
			t.Fatal("early limit")
		}
	}
	if a.allow("extra", true) {
		t.Fatal("global limit")
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.1:123"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	if a.ip(r) != "203.0.113.1" {
		t.Fatal("spoofed public proxy")
	}
	r.RemoteAddr = "127.0.0.1:123"
	if a.ip(r) != "1.2.3.4" {
		t.Fatal("trusted proxy")
	}
}

func TestHeadDoesNotSubscribe(t *testing.T) {
	s := monitor.New("test")
	a := New(s, "", false)
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, httptest.NewRequest("HEAD", "/api/v1/events", nil))
	if w.Code != 200 || s.Subscribers() != 0 || a.streams != 0 {
		t.Fatal("HEAD created a stream")
	}
}
