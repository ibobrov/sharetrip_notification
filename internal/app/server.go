package app

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/gofiber/fiber/v2"
)

func Serve(ctx context.Context, server *fiber.App, address string, shutdownTimeout time.Duration) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	return serve(ctx, server, listener, shutdownTimeout)
}

func serve(
	ctx context.Context,
	server *fiber.App,
	listener net.Listener,
	shutdownTimeout time.Duration,
) error {
	serveError := make(chan error, 1)
	go func() {
		serveError <- server.Listener(listener)
	}()

	select {
	case err := <-serveError:
		if err != nil {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := server.ShutdownWithContext(shutdownContext); err != nil {
			return fmt.Errorf("shutdown HTTP: %w", err)
		}
		if err := <-serveError; err != nil {
			return fmt.Errorf("serve HTTP after shutdown: %w", err)
		}
		return nil
	}
}
