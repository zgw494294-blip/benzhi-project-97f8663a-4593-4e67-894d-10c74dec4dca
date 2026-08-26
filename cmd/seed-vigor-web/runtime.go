package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"seed-vigor-workbench/internal/application"
	"seed-vigor-workbench/internal/persistence"
	webserver "seed-vigor-workbench/internal/web"
)

type runtime struct {
	store      *persistence.Store
	httpServer *http.Server
	listener   net.Listener
}

func buildRuntime(cfg config) (*runtime, error) {
	store, err := persistence.Open(cfg.database)
	if err != nil {
		return nil, err
	}
	service := application.NewService(store, persistence.NewID)
	web := webserver.NewServer(service, cfg.staticDir)
	listener, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("监听 %s: %w", cfg.addr, err)
	}
	return &runtime{store: store, httpServer: webserver.NewHTTPServer(cfg.addr, web.Handler()), listener: listener}, nil
}

func (r *runtime) serve() <-chan error {
	result := make(chan error, 1)
	go func() {
		err := r.httpServer.Serve(r.listener)
		if err == http.ErrServerClosed {
			err = nil
		}
		result <- err
	}()
	return result
}

func (r *runtime) shutdown(ctx context.Context) error {
	serverErr := r.httpServer.Shutdown(ctx)
	storeErr := r.store.Close()
	if serverErr != nil {
		return serverErr
	}
	return storeErr
}

func runService(cfg config) error {
	runtime, err := buildRuntime(cfg)
	if err != nil {
		return err
	}
	serveResult := runtime.serve()
	log.Printf("种子发芽势检验工作台已监听 http://%s", runtime.listener.Addr())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case err := <-serveResult:
		closeErr := runtime.store.Close()
		if err != nil {
			return err
		}
		return closeErr
	case <-signals:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return runtime.shutdown(ctx)
	}
}
