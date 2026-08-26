package main

import (
	"context"
	"errors"
	"log"
	"os/signal"
	"syscall"

	"ai-assistant/internal/app"
	"ai-assistant/internal/platform/config"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() (runErr error) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	container, err := app.NewContainer(ctx, config.Load())
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, container.Close()) }()
	return container.Run(ctx)
}
