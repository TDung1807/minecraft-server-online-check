package monitor

import (
	"context"
	"errors"
	"log/slog"
	"minecraft-monitor/internal/minecraft"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Known struct {
	PlayersOnline   int       `json:"playersOnline"`
	PlayersMax      int       `json:"playersMax"`
	Version         string    `json:"version"`
	QueryDurationMS float64   `json:"queryDurationMs"`
	ObservedAt      time.Time `json:"observedAt"`
}
type Snapshot struct {
	StaleAfterMS        int64      `json:"staleAfterMs"`
	Server              string     `json:"server"`
	State               string     `json:"state"`
	PlayersOnline       *int       `json:"playersOnline"`
	PlayersMax          *int       `json:"playersMax"`
	Version             *string    `json:"version"`
	QueryDurationMS     *float64   `json:"queryDurationMs"`
	LastAttemptAt       *time.Time `json:"lastAttemptAt"`
	LastSuccessAt       *time.Time `json:"lastSuccessAt"`
	ConsecutiveFailures int        `json:"consecutiveFailures"`
	ErrorCode           *string    `json:"errorCode"`
	LastKnown           *Known     `json:"lastKnown"`
	Sequence            uint64     `json:"sequence"`
}
type Store struct {
	mu          sync.Mutex
	snapshot    Snapshot
	subscribers map[chan Snapshot]struct{}
	Ready       atomic.Bool
	Attempts    atomic.Uint64
	Failures    atomic.Uint64
}

func New(server string, stale ...time.Duration) *Store {
	threshold := 10 * time.Second
	if len(stale) > 0 {
		threshold = stale[0]
	}
	return &Store{snapshot: Snapshot{Server: server, State: "unknown", StaleAfterMS: threshold.Milliseconds()}, subscribers: make(map[chan Snapshot]struct{})}
}
func (s *Store) Snapshot() Snapshot { s.mu.Lock(); defer s.mu.Unlock(); return s.snapshot }
func (s *Store) Subscribe() (chan Snapshot, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan Snapshot, 1)
	ch <- s.snapshot
	s.subscribers[ch] = struct{}{}
	return ch, func() { s.mu.Lock(); defer s.mu.Unlock(); delete(s.subscribers, ch) }
}
func (s *Store) Subscribers() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.subscribers) }
func (s *Store) publish() {
	s.snapshot.Sequence++
	for ch := range s.subscribers {
		select {
		case ch <- s.snapshot:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- s.snapshot:
			default:
			}
		}
	}
}
func (s *Store) invalidate(state string) {
	s.snapshot.State = state
	s.snapshot.PlayersOnline = nil
	s.snapshot.PlayersMax = nil
	s.snapshot.Version = nil
	s.snapshot.QueryDurationMS = nil
}
func (s *Store) Record(result minecraft.Result, err error, attempted, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	attempted = attempted.UTC()
	now = now.UTC()
	s.snapshot.LastAttemptAt = &attempted
	s.Attempts.Add(1)
	if err != nil {
		s.Failures.Add(1)
		s.snapshot.ConsecutiveFailures++
		code := "connection_error"
		var ne net.Error
		if errors.Is(err, minecraft.ErrInvalid) {
			code = "invalid_response"
		} else if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
			code = "timeout"
		} else {
			var dns *net.DNSError
			if errors.As(err, &dns) {
				code = "dns_error"
			}
		}
		s.snapshot.ErrorCode = &code
		state := "stale"
		if s.snapshot.ConsecutiveFailures >= 3 {
			state = "unreachable"
		}
		s.invalidate(state)
	} else {
		known := &Known{result.Online, result.Max, result.Version, result.DurationMS, now}
		s.snapshot.LastKnown = known
		s.snapshot.LastSuccessAt = &now
		s.snapshot.State = "online"
		s.snapshot.ConsecutiveFailures = 0
		s.snapshot.ErrorCode = nil
		s.snapshot.PlayersOnline = &known.PlayersOnline
		s.snapshot.PlayersMax = &known.PlayersMax
		s.snapshot.Version = &known.Version
		s.snapshot.QueryDurationMS = &known.QueryDurationMS
	}
	s.publish()
}
func (s *Store) Expire(now time.Time, threshold time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshot.State == "online" && now.Sub(*s.snapshot.LastSuccessAt) > threshold {
		code := "data_expired"
		s.snapshot.ErrorCode = &code
		s.invalidate("stale")
		s.publish()
	}
}

type Querier interface {
	Query(context.Context) (minecraft.Result, error)
}

func Run(ctx context.Context, s *Store, client Querier, interval, stale time.Duration, log *slog.Logger) {
	startedAt := time.Now()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		guard := time.NewTicker(time.Second)
		defer guard.Stop()
		alerted := false
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-guard.C:
				s.Expire(now, stale)
				snap := s.Snapshot()
				old := snap.LastSuccessAt == nil || now.Sub(*snap.LastSuccessAt) > 60*time.Second
				if old && !alerted {
					if snap.LastAttemptAt != nil && now.Sub(startedAt) > 60*time.Second {
						log.Warn("no_fresh_data", "server", snap.Server)
						alerted = true
					}
				}
				if !old {
					alerted = false
				}
			}
		}
	}()
	s.Ready.Store(true)
	defer s.Ready.Store(false)
	for {
		attempted := time.Now()
		result, err := client.Query(ctx)
		if ctx.Err() != nil {
			<-done
			return
		}
		s.Record(result, err, attempted, time.Now())
		snap := s.Snapshot()
		log.Info("minecraft_query", "state", snap.State, "duration_ms", float64(time.Since(attempted).Microseconds())/1000, "error_code", snap.ErrorCode)
		// Drain ticks accrued during a slow query rather than executing a burst.
		select {
		case <-ticker.C:
		default:
		}
		select {
		case <-ctx.Done():
			<-done
			return
		case <-ticker.C:
		}
	}
}
