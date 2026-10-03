package monitor

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"minecraft-monitor/internal/minecraft"
	"sync/atomic"
	"testing"
	"time"
)

func TestStates(t *testing.T) {
	s := New("test:1")
	if s.Snapshot().State != "unknown" || s.Snapshot().PlayersOnline != nil {
		t.Fatal("initial")
	}
	now := time.Now().UTC()
	s.Record(minecraft.Result{Online: 0, Max: 20}, nil, now, now)
	old := s.Snapshot()
	if old.State != "online" || *old.PlayersOnline != 0 {
		t.Fatal(old)
	}
	for i := 1; i <= 3; i++ {
		s.Record(minecraft.Result{}, errors.New("offline"), now, now)
		snap := s.Snapshot()
		want := "stale"
		if i == 3 {
			want = "unreachable"
		}
		if snap.State != want || snap.PlayersOnline != nil || snap.LastKnown.PlayersOnline != 0 {
			t.Fatal(snap)
		}
	}
	s.Record(minecraft.Result{Online: 3, Max: 20}, nil, now, now)
	if s.Snapshot().ConsecutiveFailures != 0 || *old.PlayersOnline != 0 {
		t.Fatal("recovery or immutable snapshot")
	}
	s.Expire(now.Add(11*time.Second), 10*time.Second)
	if s.Snapshot().State != "stale" {
		t.Fatal("expire")
	}
}
func TestPlayersFreshness(t *testing.T) {
	s := New("test")
	now := time.Now()
	players := []minecraft.Player{{Name: "Alex", ID: "uuid"}}
	s.Record(minecraft.Result{Online: 3, Players: players}, nil, now, now)
	players[0].Name = "changed"
	snap := s.Snapshot()
	if len(snap.Players) != 1 || snap.Players[0].Name != "Alex" {
		t.Fatal("player sample was not copied", snap)
	}
	s.Expire(now.Add(11*time.Second), 10*time.Second)
	if snap = s.Snapshot(); snap.Players != nil || snap.LastKnown.Players[0].Name != "Alex" {
		t.Fatal("stale player sample is current", snap)
	}
	s.Record(minecraft.Result{Online: 0}, nil, now, now)
	if snap = s.Snapshot(); snap.Players == nil || len(snap.Players) != 0 {
		t.Fatal("empty online player list", snap)
	}
	s.Record(minecraft.Result{}, errors.New("offline"), now, now)
	if s.Snapshot().Players != nil {
		t.Fatal("failed query retained current players")
	}
}

func TestLatestOnly(t *testing.T) {
	s := New("test")
	ch, cancel := s.Subscribe()
	defer cancel()
	now := time.Now()
	for i := 0; i < 100; i++ {
		s.Record(minecraft.Result{Online: i}, nil, now, now)
	}
	if snap := <-ch; snap.Sequence != 100 || *snap.PlayersOnline != 99 {
		t.Fatal(snap)
	}
	cancel()
	if s.Subscribers() != 0 {
		t.Fatal("leak")
	}
}

type fakeQuery struct{ calls atomic.Int32 }

func (f *fakeQuery) Query(context.Context) (minecraft.Result, error) {
	f.calls.Add(1)
	return minecraft.Result{Online: 1}, nil
}
func TestSinglePoller(t *testing.T) {
	s := New("test")
	f := &fakeQuery{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx, s, f, 20*time.Millisecond, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
		close(done)
	}()
	for i := 0; i < 100; i++ {
		_, unsubscribe := s.Subscribe()
		defer unsubscribe()
	}
	time.Sleep(115 * time.Millisecond)
	cancel()
	<-done
	if n := f.calls.Load(); n < 3 || n > 7 {
		t.Fatalf("unexpected queries: %d", n)
	}
	if s.Ready.Load() {
		t.Fatal("ready after stop")
	}
}
