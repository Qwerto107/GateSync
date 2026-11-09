package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gatesync/internal/app"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if len(os.Args) > 1 {
		if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
			return healthcheck()
		}
		return errors.New("用法：gatesync [healthcheck]")
	}
	cfg, err := app.LoadConfig()
	if err != nil {
		return err
	}
	application, err := app.New(cfg)
	if err != nil {
		return err
	}
	defer application.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	schedulerDone := make(chan struct{})
	go func() { defer close(schedulerDone); application.RunScheduler(ctx) }()
	server := &http.Server{Addr: cfg.Address, Handler: application.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 5 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	server.BaseContext = func(net.Listener) context.Context { return ctx }
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		if e := server.Shutdown(shutdown); e != nil {
			_ = server.Close()
		}
	}()
	log.Printf("GateSync 正在监听 %s", cfg.Address)
	if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		stop()
	}
	stop()
	<-schedulerDone
	<-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
