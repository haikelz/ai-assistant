package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

const shutdownTimeout = 10 * time.Second

type serverResult struct {
	name string
	err  error
}

type readyListener struct {
	net.Listener
	once  sync.Once
	ready chan<- struct{}
}

func (l *readyListener) Accept() (net.Conn, error) {
	l.once.Do(func() { l.ready <- struct{}{} })
	return l.Listener.Accept()
}

func (c *Container) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("run context is required")
	}
	financeListener, err := net.Listen(c.FinanceApp.Config().Network, c.Config.FinanceAddress)
	if err != nil {
		return fmt.Errorf("listen finance API: %w", err)
	}
	jobSearchListener, err := net.Listen(c.JobSearchApp.Config().Network, c.Config.JobSearchAddress)
	if err != nil {
		return errors.Join(fmt.Errorf("listen job-search API: %w", err), closeListener("finance API", financeListener))
	}

	ready := make(chan struct{}, 2)
	financeServerListener := &readyListener{Listener: financeListener, ready: ready}
	jobSearchServerListener := &readyListener{Listener: jobSearchListener, ready: ready}
	results := make(chan serverResult, 2)
	go func() {
		results <- serverResult{name: "finance API", err: c.FinanceApp.Listener(financeServerListener)}
	}()
	go func() {
		results <- serverResult{name: "job-search API", err: c.JobSearchApp.Listener(jobSearchServerListener)}
	}()

	for started := 0; started < 2; started++ {
		select {
		case <-ready:
		case result := <-results:
			shutdownErr := c.shutdownServers(ctx, financeListener, jobSearchListener)
			other := <-results
			return errors.Join(unexpectedServerExit(result), unexpectedServerExit(other), shutdownErr)
		}
	}

	var first *serverResult
	select {
	case <-ctx.Done():
	case result := <-results:
		first = &result
	}

	shutdownErr := c.shutdownServers(ctx, financeListener, jobSearchListener)
	serverErrors := make([]error, 0, 2)
	if first != nil {
		serverErrors = append(serverErrors, unexpectedServerExit(*first))
	}
	for received := 0; received < 2; received++ {
		if first != nil && received == 1 {
			break
		}
		result := <-results
		if result.err != nil && !errors.Is(result.err, net.ErrClosed) {
			serverErrors = append(serverErrors, fmt.Errorf("serve %s: %w", result.name, result.err))
		}
	}
	return errors.Join(append(serverErrors, shutdownErr)...)
}

func (c *Container) shutdownServers(parent context.Context, listeners ...net.Listener) error {
	shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(parent), shutdownTimeout)
	defer cancel()

	shutdownErrors := make(chan error, 2)
	go func() { shutdownErrors <- c.FinanceApp.ShutdownWithContext(shutdownContext) }()
	go func() { shutdownErrors <- c.JobSearchApp.ShutdownWithContext(shutdownContext) }()
	var errs []error
	for range 2 {
		if err := <-shutdownErrors; err != nil {
			errs = append(errs, err)
		}
	}
	for _, listener := range listeners {
		if err := closeListener("API", listener); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("shutdown APIs: %w", err)
	}
	return nil
}

func unexpectedServerExit(result serverResult) error {
	if result.err == nil {
		return fmt.Errorf("serve %s: server stopped unexpectedly", result.name)
	}
	if errors.Is(result.err, net.ErrClosed) {
		return nil
	}
	return fmt.Errorf("serve %s: %w", result.name, result.err)
}

func closeListener(name string, listener net.Listener) error {
	if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("close %s listener: %w", name, err)
	}
	return nil
}
