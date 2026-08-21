package app

import (
	"context"
	"fmt"
)

func (c *Container) Run(ctx context.Context) error {
	errors := make(chan error, 2)
	go func() { errors <- c.FinanceApp.Listen(c.Config.FinanceAddress) }()
	go func() { errors <- c.JobSearchApp.Listen(c.Config.JobSearchAddress) }()
	select {
	case <-ctx.Done():
		_ = c.JobSearchApp.ShutdownWithContext(context.Background())
		_ = c.FinanceApp.ShutdownWithContext(context.Background())
		return nil
	case err := <-errors:
		return fmt.Errorf("serve API: %w", err)
	}
}
