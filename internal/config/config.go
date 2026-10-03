package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host                          string
	Port                          int
	Interval, Timeout, Stale      time.Duration
	Listen, MetricsListen, Origin string
	TrustProxy                    bool
}

func Load() (Config, error) {
	c := Config{Host: env("MC_HOST", "bora.pikamc.vn"), Listen: env("HTTP_ADDR", "127.0.0.1:8080"), MetricsListen: env("METRICS_ADDR", "127.0.0.1:9090"), Origin: os.Getenv("WEB_ORIGIN")}
	var err error
	c.Port, err = strconv.Atoi(env("MC_PORT", "25005"))
	if err != nil || c.Port < 1 || c.Port > 65535 {
		return c, fmt.Errorf("invalid MC_PORT")
	}
	for _, v := range []struct {
		name, def string
		dest      *time.Duration
	}{{"POLL_INTERVAL", "5s", &c.Interval}, {"QUERY_TIMEOUT", "3s", &c.Timeout}, {"STALE_AFTER", "10s", &c.Stale}} {
		*v.dest, err = time.ParseDuration(env(v.name, v.def))
		if err != nil || *v.dest <= 0 {
			return c, fmt.Errorf("invalid %s", v.name)
		}
	}
	if c.Interval < time.Second || c.Timeout >= c.Interval || c.Stale < c.Interval {
		return c, fmt.Errorf("require interval >= 1s, timeout < interval, stale >= interval")
	}
	if c.Host == "" || len(c.Host) > 253 || strings.ContainsAny(c.Host, "/: \t\r\n") {
		return c, fmt.Errorf("MC_HOST must be a hostname or IPv4")
	}
	for _, addr := range []string{c.Listen, c.MetricsListen} {
		if _, _, err = net.SplitHostPort(addr); err != nil {
			return c, err
		}
	}
	if c.Origin != "" {
		u, e := url.Parse(c.Origin)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return c, fmt.Errorf("WEB_ORIGIN must be an http(s) origin without path")
		}
	}
	c.TrustProxy, err = strconv.ParseBool(env("TRUST_PROXY", "false"))
	return c, err
}
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
