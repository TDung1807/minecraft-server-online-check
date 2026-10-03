package minecraft

import (
	"context"
	"errors"
	pk "github.com/Tnze/go-mc/net/packet"
	"io"
	"net"
	"strconv"
	"testing"
	"time"
)

func fake(t *testing.T, action func(net.Conn)) (Client, <-chan struct{}) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	host, port, _ := net.SplitHostPort(l.Addr().String())
	n, _ := strconv.Atoi(port)
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, e := l.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(time.Second))
		var p pk.Packet
		if p.UnPack(c, -1) != nil {
			return
		}
		if p.UnPack(c, -1) != nil {
			return
		}
		action(c)
	}()
	return Client{host, n, 200 * time.Millisecond}, done
}
func TestQuery(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		valid     bool
		online    int
	}{
		{"zero", `{"players":{"online":0,"max":20},"version":{"name":"Paper"}}`, true, 0},
		{"players_without_sample", `{"players":{"online":7,"max":20}}`, true, 7},
		{"missing", `{"players":{"max":20}}`, false, 0},
		{"negative", `{"players":{"online":-1,"max":20}}`, false, 0},
		{"fraction", `{"players":{"online":1.5,"max":20}}`, false, 0},
		{"invalid_json", `oops`, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, done := fake(t, func(c net.Conn) {
				p := pk.Marshal(0, pk.String(tc.raw))
				p.Pack(c, -1)
				var b [1]byte
				_, e := c.Read(b[:])
				if e != io.EOF {
					t.Errorf("client did not close socket: %v", e)
				}
			})
			r, err := client.Query(context.Background())
			if tc.valid {
				if err != nil || r.Online != tc.online {
					t.Fatalf("%+v %v", r, err)
				}
			} else if !errors.Is(err, ErrInvalid) {
				t.Fatalf("want invalid: %v", err)
			}
			<-done
		})
	}
}
func TestTransportFailures(t *testing.T) {
	for _, mode := range []string{"oversize", "string_bomb", "timeout", "closed", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			client, done := fake(t, func(c net.Conn) {
				switch mode {
				case "oversize":
					pk.VarInt((1 << 20) + 1).WriteTo(c)
				case "string_bomb":
					pk.VarInt(6).WriteTo(c)
					c.Write([]byte{0, 255, 255, 255, 255, 7})
				case "timeout", "cancel":
					var b [1]byte
					c.Read(b[:])
				}
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				time.AfterFunc(20*time.Millisecond, cancel)
			}
			_, err := client.Query(ctx)
			if err == nil {
				t.Fatal("expected failure")
			}
			if (mode == "oversize" || mode == "string_bomb") && !errors.Is(err, ErrInvalid) {
				t.Fatal(err)
			}
			<-done
		})
	}
}
func TestDNSFailure(t *testing.T) {
	_, err := (Client{"no-such-mc-server.invalid", 25005, time.Second}).Query(context.Background())
	if err == nil {
		t.Fatal("expected DNS error")
	}
}
