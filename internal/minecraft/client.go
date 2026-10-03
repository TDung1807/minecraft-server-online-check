package minecraft

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
)

var ErrInvalid = errors.New("invalid minecraft status")

type Result struct {
	Online     int
	Max        int
	Version    string
	DurationMS float64
}
type Client struct {
	Host    string
	Port    int
	Timeout time.Duration
}

func (c Client) Query(parent context.Context) (result Result, err error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(parent, c.Timeout)
	defer cancel()
	defer func() {
		if err != nil && ctx.Err() != nil {
			err = ctx.Err()
		}
	}()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(c.Host, strconv.Itoa(c.Port)))
	if err != nil {
		return Result{}, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err = conn.SetDeadline(deadline); err != nil {
		return Result{}, err
	}
	handshake := pk.Marshal(0, pk.VarInt(767), pk.String(c.Host), pk.UnsignedShort(c.Port), pk.VarInt(1))
	if err = handshake.Pack(conn, -1); err != nil {
		return Result{}, err
	}
	request := pk.Marshal(0)
	if err = request.Pack(conn, -1); err != nil {
		return Result{}, err
	}
	reader := bufio.NewReader(conn)
	var size pk.VarInt
	if _, err = size.ReadFrom(reader); err != nil {
		return Result{}, err
	}
	if size < 2 || size > 1<<20 {
		return Result{}, fmt.Errorf("%w: packet length", ErrInvalid)
	}
	payload := make([]byte, int(size))
	if _, err = io.ReadFull(reader, payload); err != nil {
		return Result{}, err
	}
	// Validate both lengths before allocating or decoding untrusted JSON.
	body := bytes.NewReader(payload)
	var id, textSize pk.VarInt
	if _, err = id.ReadFrom(body); err != nil || id != 0 {
		return Result{}, fmt.Errorf("%w: packet id", ErrInvalid)
	}
	if _, err = textSize.ReadFrom(body); err != nil || textSize < 0 || int(textSize) != body.Len() {
		return Result{}, fmt.Errorf("%w: string length", ErrInvalid)
	}
	raw := payload[len(payload)-body.Len():]
	var data struct {
		Players *struct {
			Online *int `json:"online"`
			Max    *int `json:"max"`
		} `json:"players"`
		Version struct {
			Name string `json:"name"`
		} `json:"version"`
	}
	if err = json.Unmarshal(raw, &data); err != nil {
		return Result{}, fmt.Errorf("%w: json", ErrInvalid)
	}
	if data.Players == nil || data.Players.Online == nil || data.Players.Max == nil || *data.Players.Online < 0 || *data.Players.Max < 0 {
		return Result{}, fmt.Errorf("%w: player fields", ErrInvalid)
	}
	if len(data.Version.Name) > 256 {
		return Result{}, fmt.Errorf("%w: version name too long", ErrInvalid)
	}
	if err = ctx.Err(); err != nil {
		return Result{}, err
	}
	return Result{*data.Players.Online, *data.Players.Max, data.Version.Name, float64(time.Since(start).Microseconds()) / 1000}, nil
}
