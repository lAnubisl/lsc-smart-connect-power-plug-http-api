// Package plug serializes operations per device without caching its state.
package plug

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"plug-api/internal/config"
	"plug-api/internal/logging"
	"plug-api/internal/tuyalan"
)

type Error struct {
	Code   string
	Result string
}

func (e *Error) Error() string { return e.Code }

type device struct {
	config config.Plug
	gate   chan struct{}
	slots  chan struct{}
}

type Service struct {
	devices map[string]*device
	ids     []string
	logger  *slog.Logger
}

func New(plugs []config.Plug, logger *slog.Logger) *Service {
	s := &Service{devices: make(map[string]*device), ids: make([]string, 0, len(plugs)), logger: logger}
	for _, p := range plugs {
		s.devices[p.ID] = &device{config: p, gate: make(chan struct{}, 1), slots: make(chan struct{}, 9)}
		s.ids = append(s.ids, p.ID)
	}
	return s
}

func (s *Service) IDs() []string { return s.ids }

func (s *Service) Read(ctx context.Context, id string) (Snapshot, error) {
	return s.exchange(ctx, id, nil)
}

func (s *Service) SetRelay(ctx context.Context, id string, on bool) (Snapshot, error) {
	return s.exchange(ctx, id, map[string]any{dpRelay: on})
}

func (s *Service) SetSettings(ctx context.Context, id string, settings Settings) (Snapshot, error) {
	values, err := settings.Values()
	if err != nil {
		return Snapshot{}, err
	}
	return s.exchange(ctx, id, values)
}

func (s *Service) exchange(ctx context.Context, id string, values map[string]any) (snapshot Snapshot, err error) {
	d, exists := s.devices[id]
	if !exists {
		return snapshot, &Error{Code: "plug_not_found", Result: "failed"}
	}
	if ctx.Err() != nil {
		return snapshot, contextError(ctx)
	}
	// Nine slots allow one active operation and at most eight waiting requests.
	select {
	case d.slots <- struct{}{}:
		defer func() { <-d.slots }()
	default:
		return snapshot, &Error{Code: "plug_busy", Result: "failed"}
	}
	select {
	case d.gate <- struct{}{}:
		defer func() { <-d.gate }()
	case <-ctx.Done():
		return snapshot, contextError(ctx)
	}
	if ctx.Err() != nil {
		return snapshot, contextError(ctx)
	}
	operation := "read"
	if values != nil {
		operation = "write"
	}
	started := time.Now()
	logger := s.logger.With("request_id", logging.RequestID(ctx))
	writeAttempted := false
	defer func() {
		if err != nil {
			var failure *Error
			if !errors.As(err, &failure) {
				failure = &Error{Code: tuyalan.SafeError(err), Result: "failed"}
				if ctx.Err() != nil {
					failure = contextError(ctx)
				}
			}
			if writeAttempted {
				failure.Result = "unknown"
			}
			err = failure
			logger.WarnContext(ctx, "device operation failed", "plug_id", id, "operation", operation, "code", failure.Code, "result", failure.Result, "duration_ms", time.Since(started).Milliseconds())
		} else {
			logger.InfoContext(ctx, "device operation completed", "plug_id", id, "operation", operation, "duration_ms", time.Since(started).Milliseconds())
		}
	}()
	client, err := tuyalan.Connect(ctx, d.config.Address, d.config.LocalKey)
	if err != nil {
		return snapshot, err
	}
	defer client.Close()
	if values != nil {
		// Validate support using a fresh device response before any write occurs.
		before, readErr := client.Status()
		if readErr != nil {
			return snapshot, readErr
		}
		for dp, expected := range values {
			if !sameType(before[dp], expected) {
				return snapshot, &Error{Code: "unsupported_setting", Result: "failed"}
			}
		}
		if ctx.Err() != nil {
			return snapshot, contextError(ctx)
		}
		writeAttempted = true
		if err = client.Control(values); err != nil {
			return snapshot, err
		}
	}
	dps, err := client.Status()
	if err != nil {
		return snapshot, err
	}
	snapshot = Snapshot{ID: id, ReceivedAt: time.Now().UTC(), DPS: dps}
	if values != nil {
		for dp, expected := range values {
			if !confirmed(dps[dp], expected, dp) {
				return snapshot, &Error{Code: "verification_failed", Result: "unknown"}
			}
		}
	}
	return snapshot, nil
}

func contextError(ctx context.Context) *Error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return &Error{Code: "request_canceled", Result: "failed"}
	}
	return &Error{Code: "device_timeout", Result: "failed"}
}

func sameType(raw json.RawMessage, expected any) bool {
	var actual any
	if json.Unmarshal(raw, &actual) != nil {
		return false
	}
	switch expected.(type) {
	case bool:
		_, ok := actual.(bool)
		return ok
	case string:
		_, ok := actual.(string)
		return ok
	case int:
		_, ok := actual.(float64)
		return ok
	}
	return false
}

func confirmed(raw json.RawMessage, expected any, dp string) bool {
	var actual any
	if json.Unmarshal(raw, &actual) != nil {
		return false
	}
	switch v := expected.(type) {
	case bool:
		return actual == v
	case string:
		return actual == v
	case int:
		number, ok := actual.(float64)
		if !ok {
			return false
		}
		// A countdown can decrease between writing it and reading it back.
		if dp == dpCountdownSeconds && v > 0 {
			return number > 0 && number <= float64(v) && number >= float64(v)-2
		}
		return number == float64(v)
	}
	return false
}
