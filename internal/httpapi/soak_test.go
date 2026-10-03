package httpapi

import (
	"bufio"
	"context"
	"fmt"
	pk "github.com/Tnze/go-mc/net/packet"
	"io"
	"log/slog"
	"minecraft-monitor/internal/minecraft"
	"minecraft-monitor/internal/monitor"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Opt-in soak test exercises real TCP Minecraft framing and 100 HTTP streams.
func TestSoak(t *testing.T) {
	durationText := os.Getenv("SOAK_DURATION")
	if durationText == "" {
		t.Skip("set SOAK_DURATION=30m to run the full acceptance soak")
	}
	duration, err := time.ParseDuration(durationText)
	if err != nil || duration < 10*time.Second {
		t.Fatal("SOAK_DURATION must be >= 10s")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var calls atomic.Int64
	var sockets sync.WaitGroup
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			c, e := listener.Accept()
			if e != nil {
				return
			}
			sockets.Add(1)
			go func() {
				defer sockets.Done()
				defer c.Close()
				c.SetDeadline(time.Now().Add(3 * time.Second))
				var p pk.Packet
				if p.UnPack(c, -1) != nil || p.UnPack(c, -1) != nil {
					return
				}
				calls.Add(1)
				response := pk.Marshal(0, pk.String(`{"players":{"online":7,"max":20},"version":{"name":"Fake Paper"}}`))
				response.Pack(c, -1)
				io.Copy(io.Discard, c)
			}()
		}
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	portNum, _ := strconv.Atoi(port)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := monitor.New("fake")
	pollDone := make(chan struct{})
	go func() {
		defer close(pollDone)
		monitor.Run(ctx, store, minecraft.Client{Host: host, Port: portNum, Timeout: time.Second}, 5*time.Second, 10*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	srv := httptest.NewServer(New(store, "", true).Handler())
	defer srv.Close()
	client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 110}}
	var streams []*http.Response
	var readers sync.WaitGroup
	var events atomic.Int64
	defer func() {
		for _, r := range streams {
			r.Body.Close()
		}
		readers.Wait()
		cancel()
		<-pollDone
		listener.Close()
		<-stopped
		sockets.Wait()
		client.CloseIdleConnections()
	}()
	for i := 0; i < 100; i++ {
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/events", nil)
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.1.0.%d", i+1))
		resp, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			t.Fatal(resp.StatusCode)
		}
		streams = append(streams, resp)
		readers.Add(1)
		go func() {
			defer readers.Done()
			scan := bufio.NewScanner(resp.Body)
			for scan.Scan() {
				if len(scan.Text()) >= 6 && scan.Text()[:6] == "data: " {
					events.Add(1)
				}
			}
		}()
	}
	time.Sleep(time.Second)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	deadline := start.Add(duration)
	var latencies []time.Duration
	index := 0
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/v1/status", nil)
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.2.%d.%d", (index/250)%250, index%250+1))
		index++
		at := time.Now()
		resp, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		latencies = append(latencies, time.Since(at))
		if resp.StatusCode != 200 {
			t.Fatal(resp.StatusCode)
		}
		time.Sleep(50 * time.Millisecond)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p95 := latencies[len(latencies)*95/100]
	expected := int64((time.Since(start)+time.Second)/(5*time.Second)) + 2
	if calls.Load() > expected {
		t.Fatalf("Minecraft amplified by viewers: %d > %d", calls.Load(), expected)
	}
	if minimum := int64(duration/(5*time.Second)) - 2; calls.Load() < minimum {
		t.Fatalf("poller missed too many cycles: %d < %d", calls.Load(), minimum)
	}
	if snap := store.Snapshot(); snap.State != "online" || snap.LastSuccessAt == nil || time.Since(*snap.LastSuccessAt) > 10*time.Second {
		t.Fatal("status stopped updating", snap)
	}
	if p95 > 100*time.Millisecond {
		t.Fatalf("REST p95 %s", p95)
	}
	growth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	if growth > 8<<20 {
		t.Fatalf("heap growth %d bytes", growth)
	}
	if events.Load() < 200 {
		t.Fatal("missing live updates", events.Load())
	}
	t.Logf("duration=%s SSE=100 queries=%d REST=%d p95=%s heap_growth=%d bytes events=%d", duration, calls.Load(), len(latencies), p95, growth, events.Load())
}
