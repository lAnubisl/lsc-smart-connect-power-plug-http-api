// Package logging sends the same slog events to stdout and optional OTLP.
package logging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"plug-api/internal/config"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

func New(ctx context.Context, cfg config.Logging) (*slog.Logger, func(context.Context) error, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		return nil, nil, err
	}
	local := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	handlers := []slog.Handler{local}
	var provider *sdklog.LoggerProvider
	if cfg.OTel.Enabled {
		exporter, err := otlploghttp.New(ctx,
			otlploghttp.WithEndpointURL(cfg.OTel.Endpoint),
			otlploghttp.WithURLPath("/v1/logs"),
			otlploghttp.WithTimeout(5*time.Second),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("initialize OTLP log exporter")
		}
		// A bounded queue decouples HTTP requests from collector availability.
		processor := sdklog.NewBatchProcessor(exporter,
			sdklog.WithMaxQueueSize(1024),
			sdklog.WithExportMaxBatchSize(128),
			sdklog.WithExportInterval(2*time.Second),
			sdklog.WithExportTimeout(5*time.Second),
		)
		provider = sdklog.NewLoggerProvider(
			sdklog.WithProcessor(processor),
			sdklog.WithResource(resource.NewSchemaless(attribute.String("service.name", cfg.OTel.ServiceName))),
		)
		handlers = append(handlers, otelslog.NewHandler("plug-api", otelslog.WithLoggerProvider(provider)))
		// Export failures go only to the local handler, preventing a logging loop.
		otel.SetErrorHandler(otel.ErrorHandlerFunc(func(error) {
			slog.New(local).Warn("OpenTelemetry export failed; remote records may be dropped")
		}))
	}
	logger := slog.New(&fanout{handlers: handlers, level: level})
	shutdown := func(ctx context.Context) error {
		var exportErr error
		if provider != nil {
			exportErr = provider.Shutdown(ctx)
		}
		return exportErr
	}
	return logger, shutdown, nil
}

// fanout is only a slog adapter: application code always uses *slog.Logger.
type fanout struct {
	handlers []slog.Handler
	level    slog.Level
}

func (h *fanout) Enabled(_ context.Context, level slog.Level) bool { return level >= h.level }

func (h *fanout) Handle(ctx context.Context, record slog.Record) error {
	var result error
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, record.Level) {
			result = errors.Join(result, handler.Handle(ctx, record.Clone()))
		}
	}
	if result != nil {
		fmt.Fprintln(os.Stderr, "plug-api: failed to write a log record")
	}
	return result
}

func (h *fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	copy := &fanout{level: h.level}
	for _, handler := range h.handlers {
		copy.handlers = append(copy.handlers, handler.WithAttrs(attrs))
	}
	return copy
}

func (h *fanout) WithGroup(name string) slog.Handler {
	copy := &fanout{level: h.level}
	for _, handler := range h.handlers {
		copy.handlers = append(copy.handlers, handler.WithGroup(name))
	}
	return copy
}
