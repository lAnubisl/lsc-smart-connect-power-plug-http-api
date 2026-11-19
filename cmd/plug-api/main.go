package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"plug-api/internal/config"
	"plug-api/internal/httpapi"
	"plug-api/internal/ledstrip"
	"plug-api/internal/logging"
	"plug-api/internal/smartplug"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "plug-api:", err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("config", "config.json", "Path to the JSON configuration file")
	check := flag.Bool("check-config", false, "Validate configuration and exit without contacting devices")
	flag.Parse()
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	if *check {
		fmt.Printf("Configuration valid: %d plugs, %d lights\n", len(cfg.Plugs), len(cfg.LEDStrips))
		return nil
	}
	logger, closeLogs, err := logging.New(context.Background(), cfg.Logging)
	if err != nil {
		return err
	}
	slog.SetDefault(logger)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		if err := closeLogs(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "plug-api: log shutdown failed")
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	plugs := smartplug.New(cfg.Plugs, logger)
	strips := ledstrip.New(cfg.LEDStrips, logger)
	server := &http.Server{
		Addr:              cfg.HTTP.Listen,
		Handler:           httpapi.New(cfg.HTTP, plugs, strips, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      time.Duration(cfg.HTTP.RequestTimeoutSeconds+10) * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	logger.Info("service started", "listen", cfg.HTTP.Listen, "plug_count", len(cfg.Plugs), "light_count", len(cfg.LEDStrips))
	select {
	case err := <-result:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		logger.Info("service stopping")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.HTTP.RequestTimeoutSeconds+2)*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			server.Close()
			return err
		}
	}
	logger.Info("service stopped")
	return nil
}
