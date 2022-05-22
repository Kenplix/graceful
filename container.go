package graceful

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
)

type Runner interface {
	Run(ctx context.Context) error
}

func Run(ctx context.Context, r Runner) error {
	failure := make(chan error)
	go func() { failure <- r.Run(ctx) }()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-failure:
			return err
		}
	}
}

type Stoppable interface {
	Container() *Container
}

func Shutdown(ctx context.Context, s Stoppable) error {
	return s.Container().Shutdown(ctx)
}

type Container struct {
	breakers []breaker
	mu       sync.Mutex
	once     sync.Once
}

type Func func(ctx context.Context) error

type breaker struct {
	name string
	fn   Func
}

func (c *Container) Add(name string, fn Func) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.breakers = append(c.breakers, breaker{
		name: name,
		fn:   fn,
	})
}

func (c *Container) Shutdown(ctx context.Context) (err error) {
	c.once.Do(func() { err = c.shutdown(ctx) })
	return err
}

type ShutdownError struct {
	Messages []string
}

func (e ShutdownError) Error() string {
	return fmt.Sprintf("shutdown finished with error(s): \n%s", strings.Join(e.Messages, "\n"))
}

func (c *Container) shutdown(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var (
		messages = make([]string, 0, len(c.breakers))
		complete = make(chan struct{}, 1)
	)

	go func() {
		for i := len(c.breakers) - 1; i >= 0; i-- {
			log.Printf("shutting down %q gracefully", c.breakers[i].name)

			if err := c.breakers[i].fn(ctx); err != nil {
				messages = append(messages, fmt.Sprintf("[!] %v", err))
			}

			log.Printf("shutting down %q complete", c.breakers[i].name)
		}

		c.breakers = c.breakers[0:0]
		complete <- struct{}{}
	}()

	select {
	case <-complete:
		break
	case <-ctx.Done():
		return fmt.Errorf("shutdown cancelled: %v", ctx.Err())
	}

	if len(messages) > 0 {
		return ShutdownError{Messages: messages}
	}

	return nil
}
