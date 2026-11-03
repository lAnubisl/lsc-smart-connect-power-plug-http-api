package plug

import (
	"encoding/json"
	"fmt"
	"time"
)

// Pointers distinguish a missing measurement from a real zero or false value.
type State struct {
	ID             string    `json:"id"`
	ReceivedAt     time.Time `json:"received_at"`
	RelayOn        *bool     `json:"relay_on"`
	CurrentA       *float64  `json:"current_a"`
	PowerW         *float64  `json:"power_w"`
	VoltageV       *float64  `json:"voltage_v"`
	EnergyReportWh *float64  `json:"energy_report_wh"`
	Fault          *uint32   `json:"fault"`
}

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

type Snapshot struct {
	ID         string                     `json:"id"`
	ReceivedAt time.Time                  `json:"received_at"`
	DPS        map[string]json.RawMessage `json:"dps"`
}

func (s Snapshot) State() (State, error) {
	var state State
	state.ID, state.ReceivedAt = s.ID, s.ReceivedAt
	var err error
	if state.RelayOn, err = field[bool](s.DPS, dpRelay); err != nil {
		return state, err
	}
	if state.CurrentA, err = scaled(s.DPS, dpCurrent, 1000); err != nil {
		return state, err
	}
	if state.PowerW, err = scaled(s.DPS, dpPower, 10); err != nil {
		return state, err
	}
	if state.VoltageV, err = scaled(s.DPS, dpVoltage, 10); err != nil {
		return state, err
	}
	if state.EnergyReportWh, err = scaled(s.DPS, dpEnergyReportWh, 1); err != nil {
		return state, err
	}
	state.Fault, err = field[uint32](s.DPS, dpFault)
	return state, err
}

func (s Snapshot) Settings() (SettingsResponse, error) {
	response := SettingsResponse{ID: s.ID, ReceivedAt: s.ReceivedAt}
	v := &response.Settings
	var err error
	if v.CountdownSeconds, err = field[int](s.DPS, dpCountdownSeconds); err != nil {
		return response, err
	}
	if v.PowerOnState, err = field[string](s.DPS, dpPowerOnState); err != nil {
		return response, err
	}
	if v.OverchargeProtection, err = field[bool](s.DPS, dpOverchargeProtection); err != nil {
		return response, err
	}
	if v.IndicatorMode, err = field[string](s.DPS, dpIndicatorMode); err != nil {
		return response, err
	}
	v.ChildLock, err = field[bool](s.DPS, dpChildLock)
	return response, err
}

// T is the expected Go type (bool, string, or number). A missing DPS returns
// nil so the HTTP response can show null instead of inventing a value.
func field[T any](dps map[string]json.RawMessage, id string) (*T, error) {
	raw, exists := dps[id]
	if !exists || string(raw) == "null" {
		return nil, nil
	}
	var value T
	if json.Unmarshal(raw, &value) != nil {
		return nil, &Error{Code: "invalid_device_response", Result: "failed"}
	}
	return &value, nil
}

func scaled(dps map[string]json.RawMessage, id string, divisor float64) (*float64, error) {
	value, err := field[float64](dps, id)
	if value != nil {
		*value /= divisor
	}
	return value, err
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
