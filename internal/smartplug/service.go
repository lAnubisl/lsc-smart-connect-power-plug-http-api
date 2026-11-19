// Package smartplug controls the relay and reads measurements from LSC smart plugs.
package smartplug

import (
	"context"
	"log/slog"

	"plug-api/internal/config"
	"plug-api/internal/device"
)

type Service struct {
	devices *device.Service
}

func New(devices []config.Device, logger *slog.Logger) *Service {
	verification := device.Verification{CountdownDPS: dpCountdownSeconds}
	return &Service{devices: device.New(devices, "plug", verification, logger)}
}

func (s *Service) IDs() []string { return s.devices.IDs() }

func (s *Service) Read(ctx context.Context, id string) (device.Snapshot, error) {
	return s.devices.Read(ctx, id)
}

func (s *Service) SetRelay(ctx context.Context, id string, on bool) (device.Snapshot, error) {
	return s.write(ctx, id, map[string]any{dpRelay: on})
}

func (s *Service) SetSettings(ctx context.Context, id string, settings Settings) (device.Snapshot, error) {
	values, err := settings.Values()
	if err != nil {
		return device.Snapshot{}, err
	}
	return s.write(ctx, id, values)
}

func (s *Service) write(ctx context.Context, id string, values map[string]any) (device.Snapshot, error) {
	return s.devices.Exchange(ctx, id, func(device.Snapshot) (device.WritePlan, error) {
		return device.WritePlan{State: values}, nil
	})
}
