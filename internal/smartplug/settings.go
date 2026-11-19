package smartplug

import (
	"fmt"
	"time"

	"plug-api/internal/device"
)

// Settings is also the PATCH request model. Only non-nil fields are written.
type Settings struct {
	CountdownSeconds     *int    `json:"countdown_seconds"`
	PowerOnState         *string `json:"power_on_state"`
	OverchargeProtection *bool   `json:"overcharge_protection"`
	IndicatorMode        *string `json:"indicator_mode"`
	ChildLock            *bool   `json:"child_lock"`
}

type SettingsResponse struct {
	ID         string    `json:"id"`
	ReceivedAt time.Time `json:"received_at"`
	Settings   Settings  `json:"settings"`
}

func DecodeSettings(s device.Snapshot) (SettingsResponse, error) {
	response := SettingsResponse{ID: s.ID, ReceivedAt: s.ReceivedAt}
	v := &response.Settings
	var err error
	if v.CountdownSeconds, err = device.Field[int](s.DPS, dpCountdownSeconds); err != nil {
		return response, err
	}
	if v.PowerOnState, err = device.Field[string](s.DPS, dpPowerOnState); err != nil {
		return response, err
	}
	if v.OverchargeProtection, err = device.Field[bool](s.DPS, dpOverchargeProtection); err != nil {
		return response, err
	}
	if v.IndicatorMode, err = device.Field[string](s.DPS, dpIndicatorMode); err != nil {
		return response, err
	}
	v.ChildLock, err = device.Field[bool](s.DPS, dpChildLock)
	return response, err
}

func (s Settings) Values() (map[string]any, error) {
	values := make(map[string]any)
	if s.CountdownSeconds != nil {
		if *s.CountdownSeconds < 0 || *s.CountdownSeconds > 86400 {
			return nil, fmt.Errorf("countdown_seconds must be between 0 and 86400")
		}
		values[dpCountdownSeconds] = *s.CountdownSeconds
	}
	if s.PowerOnState != nil {
		switch *s.PowerOnState {
		case "off", "on", "memory":
			values[dpPowerOnState] = *s.PowerOnState
		default:
			return nil, fmt.Errorf("power_on_state must be off, on, or memory")
		}
	}
	if s.IndicatorMode != nil {
		switch *s.IndicatorMode {
		case "relay", "pos", "none", "on":
			values[dpIndicatorMode] = *s.IndicatorMode
		default:
			return nil, fmt.Errorf("indicator_mode must be relay, pos, none, or on")
		}
	}
	if s.OverchargeProtection != nil {
		values[dpOverchargeProtection] = *s.OverchargeProtection
	}
	if s.ChildLock != nil {
		values[dpChildLock] = *s.ChildLock
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("provide at least one setting")
	}
	return values, nil
}
