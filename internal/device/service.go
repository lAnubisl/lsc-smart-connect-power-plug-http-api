// Package device runs serialized Tuya LAN operations without caching device state.
package device

import (
	"context"
	"errors"
	"log/slog"
	"maps"
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

type endpoint struct {
	config config.Device
	gate   chan struct{}
	slots  chan struct{}
}

type Service struct {
	devices      map[string]*endpoint
	ids          []string
	group        string
	verification Verification
	logger       *slog.Logger
}

func New(devices []config.Device, group string, verification Verification, logger *slog.Logger) *Service {
	s := &Service{
		devices: make(map[string]*endpoint), ids: make([]string, 0, len(devices)),
		group: group, verification: verification, logger: logger,
	}
	for _, p := range devices {
		s.devices[p.ID] = &endpoint{config: p, gate: make(chan struct{}, 1), slots: make(chan struct{}, 9)}
		s.ids = append(s.ids, p.ID)
	}
	return s
}

func (s *Service) IDs() []string { return s.ids }

func (s *Service) Read(ctx context.Context, id string) (Snapshot, error) {
	return s.Exchange(ctx, id, nil)
}

type WritePlan struct {
	State       map[string]any   // Values verified by STATUS.
	ControlOnly map[string]any   // Sent with State and acknowledged by CONTROL.
	Commands    []map[string]any // Sent after State verification, in order.
}

func (s *Service) Exchange(ctx context.Context, id string, buildPlan func(Snapshot) (WritePlan, error)) (snapshot Snapshot, err error) {
	d, exists := s.devices[id]
	if !exists {
		return snapshot, &Error{Code: s.group + "_not_found", Result: "failed"}
	}
	if ctx.Err() != nil {
		return snapshot, contextError(ctx)
	}
	// Nine slots allow one active operation and at most eight waiting requests.
	select {
	case d.slots <- struct{}{}:
		defer func() { <-d.slots }()
	default:
		return snapshot, &Error{Code: s.group + "_busy", Result: "failed"}
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
	if buildPlan != nil {
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
			logger.WarnContext(ctx, "device operation failed", s.group+"_id", id, "operation", operation, "code", failure.Code, "result", failure.Result, "duration_ms", time.Since(started).Milliseconds())
		} else {
			logger.InfoContext(ctx, "device operation completed", s.group+"_id", id, "operation", operation, "duration_ms", time.Since(started).Milliseconds())
		}
	}()
	client, err := tuyalan.Connect(ctx, d.config.Address, d.config.LocalKey)
	if err != nil {
		return snapshot, err
	}
	defer client.Close()
	snapshot, err = readSnapshot(client, id)
	if err != nil || buildPlan == nil {
		return snapshot, err
	}

	plan, err := buildPlan(snapshot)
	if err != nil {
		return snapshot, err
	}
	snapshot, writeAttempted, err = s.applyWritePlan(ctx, client, snapshot, plan)
	return snapshot, err
}

func (s *Service) applyWritePlan(ctx context.Context, client *tuyalan.Client, snapshot Snapshot, plan WritePlan) (Snapshot, bool, error) {
	for dp, expected := range plan.State {
		if !sameType(snapshot.DPS[dp], expected) {
			return snapshot, false, &Error{Code: "unsupported_setting", Result: "failed"}
		}
	}
	writeAttempted := false
	if len(plan.State) > 0 {
		if ctx.Err() != nil {
			return snapshot, false, contextError(ctx)
		}
		control := plan.State
		if len(plan.ControlOnly) > 0 {
			control = maps.Clone(plan.State)
			maps.Copy(control, plan.ControlOnly)
		}
		writeAttempted = true
		if err := client.Control(control); err != nil {
			return snapshot, writeAttempted, err
		}
		verified, err := s.verifyState(ctx, client, snapshot.ID, plan.State)
		if err != nil {
			return verified, writeAttempted, err
		}
		snapshot = verified
	}
	for _, command := range plan.Commands {
		if ctx.Err() != nil {
			return snapshot, writeAttempted, contextError(ctx)
		}
		writeAttempted = true
		if err := client.Control(command); err != nil {
			return snapshot, writeAttempted, err
		}
	}
	return snapshot, writeAttempted, nil
}

func contextError(ctx context.Context) *Error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return &Error{Code: "request_canceled", Result: "failed"}
	}
	return &Error{Code: "device_timeout", Result: "failed"}
}
