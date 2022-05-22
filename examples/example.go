package main

import (
	"context"
	"errors"
	"fmt"
	"graceful"
	"log"
	"net"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type App struct {
	server *http.Server
}

func NewApp(ctx context.Context, addr string) *App {
	return &App{
		server: &http.Server{
			Addr: addr,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := r.Context()

				for {
					select {
					case <-ctx.Done():
						log.Printf("shutting down connection with %s gracefully", r.RemoteAddr)
						w.WriteHeader(http.StatusOK)
						return
					case <-time.After(time.Second):
						log.Printf("connection with %s alive", r.RemoteAddr)
					}
				}
			}),
			BaseContext: func(_ net.Listener) context.Context {
				return ctx
			},
		},
	}
}

func (a *App) Run(_ context.Context) error {
	if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen and serve: %w", err)
	}

	return nil
}

var (
	once      sync.Once
	container graceful.Container
)

func (a *App) Container() *graceful.Container {
	once.Do(func() {
		for _, entry := range []struct {
			name string
			fn   graceful.Func
		}{
			{
				name: "server",
				fn:   a.server.Shutdown,
			},
			{
				name: "first failing closing",
				fn: func(ctx context.Context) error {
					return errors.New("uh-oh, another error occurred")
				},
			},
			{
				name: "second failing closing",
				fn: func(ctx context.Context) error {
					return errors.New("oops error occurred")
				},
			},
		} {
			container.Add(entry.name, entry.fn)
		}
	})

	return &container
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	app := NewApp(ctx, ":80")

	defer func(ctx context.Context) {
		if err := graceful.Shutdown(ctx, app); err != nil {
			log.Println(err)
		}
	}(context.TODO())

	if err := graceful.Run(ctx, app); err != nil {
		log.Printf("exit reason: %s", err)
	}
}
