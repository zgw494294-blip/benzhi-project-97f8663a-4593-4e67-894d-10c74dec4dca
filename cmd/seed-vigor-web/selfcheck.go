package main

import (
	"context"
	"fmt"
	"os"
	"time"

	webserver "seed-vigor-workbench/internal/web"
)

func runSelfcheck(cfg config) error {
	temp, err := os.CreateTemp("", "seed-vigor-selfcheck-*.db")
	if err != nil {
		return fmt.Errorf("创建自检数据库: %w", err)
	}
	path := temp.Name()
	if err := temp.Close(); err != nil {
		return err
	}
	defer os.Remove(path)
	defer os.Remove(path + "-wal")
	defer os.Remove(path + "-shm")
	cfg.database = path
	runtime, err := buildRuntime(cfg)
	if err != nil {
		return err
	}
	serveResult := runtime.serve()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	baseURL := "http://" + runtime.listener.Addr().String()
	checkErr := webserver.RunSelfcheck(ctx, baseURL)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	shutdownErr := runtime.shutdown(shutdownCtx)
	serveErr := <-serveResult
	if checkErr != nil {
		return checkErr
	}
	if shutdownErr != nil {
		return shutdownErr
	}
	if serveErr != nil {
		return serveErr
	}
	fmt.Println("selfcheck 通过：真实 HTTP 流程已完成建档、冻结、观察、封存、批准与不可变归档")
	return nil
}
