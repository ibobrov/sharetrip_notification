package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ibobrov/sharetrip_notification/internal/api"
	"github.com/ibobrov/sharetrip_notification/internal/app"
	"github.com/ibobrov/sharetrip_notification/internal/config"
	"github.com/ibobrov/sharetrip_notification/internal/repository"
	"github.com/ibobrov/sharetrip_notification/internal/service"
	"github.com/joho/godotenv"
)

const shutdownTimeout = 5 * time.Second

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	logger := app.NewLogger(os.Stdout)
	exitCode := execute(ctx, logger)
	stop()
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func execute(ctx context.Context, logger *slog.Logger) int {
	startedAt := time.Now()
	if err := run(ctx, logger); err != nil {
		logger.Error("application stopped",
			slog.String("operation", "run"),
			slog.String("result", "error"),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
			slog.Any("error", err),
		)
		return 1
	}

	logger.Info("application stopped",
		slog.String("operation", "shutdown"),
		slog.String("result", "success"),
		slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
	)
	return 0
}

func run(ctx context.Context, logger *slog.Logger) error {
	if err := loadEnv(); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	pool, err := repository.NewPool(ctx, cfg.DB)
	if err != nil {
		return errors.New("connect to database")
	}
	defer pool.Close()

	notificationRepository := repository.NewNotificationRepository()
	notificationService := service.NewNotificationService(pool, notificationRepository)
	server := api.NewRouter(notificationService, logger)
	address := net.JoinHostPort(cfg.HTTP.Host, strconv.Itoa(cfg.HTTP.Port))

	logger.Info("application started",
		slog.String("operation", "start"),
		slog.String("result", "success"),
		slog.String("address", address),
	)

	return app.Serve(ctx, server, address, shutdownTimeout)
}

func loadEnv() error {
	err := godotenv.Load()
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return errors.New("load .env")
}
