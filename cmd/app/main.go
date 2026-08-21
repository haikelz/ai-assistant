package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"ai-assistant/internal/app"
	"ai-assistant/internal/platform/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	container, err := app.NewContainer(ctx, config.Load())
	if err != nil {
		log.Fatal(err)
	}
	defer container.Close()
	if err := container.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
