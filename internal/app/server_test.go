package app

import (
	"context"
	"net"
	"strings"
	"testing"

	"ai-assistant/internal/platform/config"
	"github.com/gofiber/fiber/v2"
)

func TestContainerRunStopsBothServersOnCancellation(t *testing.T) {
	financeAddress := availableAddress(t)
	jobSearchAddress := availableAddress(t)
	container := &Container{
		Config:       config.Config{FinanceAddress: financeAddress, JobSearchAddress: jobSearchAddress},
		FinanceApp:   fiber.New(fiber.Config{DisableStartupMessage: true}),
		JobSearchApp: fiber.New(fiber.Config{DisableStartupMessage: true}),
	}
	ctx, cancel := context.WithCancel(t.Context())
	container.FinanceApp.Hooks().OnListen(func(fiber.ListenData) error {
		cancel()
		return nil
	})
	if err := container.Run(ctx); err != nil {
		t.Fatal(err)
	}
	assertAddressAvailable(t, financeAddress)
	assertAddressAvailable(t, jobSearchAddress)
}

func TestContainerRunClosesFirstListenerWhenSecondAddressIsInvalid(t *testing.T) {
	financeAddress := availableAddress(t)
	container := &Container{
		Config:       config.Config{FinanceAddress: financeAddress, JobSearchAddress: "invalid-address"},
		FinanceApp:   fiber.New(fiber.Config{DisableStartupMessage: true}),
		JobSearchApp: fiber.New(fiber.Config{DisableStartupMessage: true}),
	}
	err := container.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "listen job-search API") {
		t.Fatalf("error=%v", err)
	}
	assertAddressAvailable(t, financeAddress)
}

func availableAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func assertAddressAvailable(t *testing.T, address string) {
	t.Helper()
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("listen %s after shutdown: %v", address, err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
}
