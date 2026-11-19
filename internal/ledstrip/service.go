// Package ledstrip controls LSC Smart Connect LED strips.
package ledstrip

import (
	"context"
	"log/slog"
	"time"

	"plug-api/internal/config"
	"plug-api/internal/device"
)

type Service struct {
	devices *device.Service
}

func New(devices []config.Device, logger *slog.Logger) *Service {
	verification := device.Verification{
		CountdownDPS:       dpCountdown,
		CaseInsensitiveDPS: dpColor,
		RetryDelay:         300 * time.Millisecond,
	}
	return &Service{devices: device.New(devices, "light", verification, logger)}
}

func (s *Service) IDs() []string { return s.devices.IDs() }

func (s *Service) Read(ctx context.Context, id string) (device.Snapshot, error) {
	return s.devices.Read(ctx, id)
}

func (s *Service) PatchState(ctx context.Context, id string, patch Patch) (device.Snapshot, error) {
	if err := patch.Validate(); err != nil {
		return device.Snapshot{}, err
	}
	return s.devices.Exchange(ctx, id, patch.plan)
}

func (s *Service) StartAnimation(ctx context.Context, id string, animation Animation) (device.Snapshot, error) {
	if err := animation.Validate(); err != nil {
		return device.Snapshot{}, err
	}
	return s.devices.Exchange(ctx, id, animation.plan)
}
