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
	"syscall"
	"time"
)

type Service struct {
	*http.Server
}

func NewService(ctx context.Context, addr string) *Service {
	return &Service{
		Server: &http.Server{
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

func (s *Service) Run(ctx context.Context) error {
	failure := make(chan error)
	go func() {
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			failure <- fmt.Errorf("listen and serve: %w", err)
		}
	}()

	log.Printf("listening on %s", s.Addr)

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-failure:
			return err
		case <-time.After(time.Second):
			log.Printf("service at %s alive", s.Addr)
		}
	}
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	svc := NewService(ctx, "127.0.0.1:8080")

	var ctr graceful.Container
	ctr.Add("service", svc.Shutdown)

	ctr.Add("first failing closing", func(ctx context.Context) error {
		return errors.New("oops error occurred")
	})

	ctr.Add("second failing closing", func(ctx context.Context) error {
		return errors.New("uh-oh, another error occurred")
	})

	defer func(ctx context.Context) {
		if err := ctr.Shutdown(ctx); err != nil {
			log.Println(err)
		}
	}(context.TODO())

	if err := svc.Run(ctx); err != nil {
		log.Printf("exit reason: %s", err)
	}
}
