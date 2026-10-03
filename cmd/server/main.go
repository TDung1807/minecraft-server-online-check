package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"minecraft-monitor/internal/config"
	"minecraft-monitor/internal/httpapi"
	"minecraft-monitor/internal/minecraft"
	"minecraft-monitor/internal/monitor"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server_failed", "error", err)
		os.Exit(1)
	}
}
func run() error {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store := monitor.New(net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.Port)), cfg.Stale)
	api := httpapi.New(store, cfg.Origin, cfg.TrustProxy)
	public := &http.Server{Addr: cfg.Listen, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
	metrics := &http.Server{Addr: cfg.MetricsListen, Handler: api.Metrics(), ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	ml, err := net.Listen("tcp", cfg.MetricsListen)
	if err != nil {
		return err
	}
	defer ml.Close()
	pollDone := make(chan struct{})
	go func() {
		defer close(pollDone)
		monitor.Run(ctx, store, minecraft.Client{Host: cfg.Host, Port: cfg.Port, Timeout: cfg.Timeout}, cfg.Interval, cfg.Stale, log)
	}()
	failed := make(chan error, 2)
	go func() { failed <- public.Serve(listener) }()
	go func() { failed <- metrics.Serve(ml) }()
	log.Info("listening", "http", cfg.Listen, "metrics", cfg.MetricsListen, "minecraft", store.Snapshot().Server)
	select {
	case <-ctx.Done():
	case err = <-failed:
		cancel()
	}
	shutdown, done := context.WithTimeout(context.Background(), 10*time.Second)
	defer done()
	if e := public.Shutdown(shutdown); e != nil {
		_ = public.Close()
		if err == nil {
			err = e
		}
	}
	if e := metrics.Shutdown(shutdown); e != nil {
		_ = metrics.Close()
		if err == nil {
			err = e
		}
	}
	<-pollDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
