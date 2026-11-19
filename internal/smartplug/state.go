package smartplug

import (
	"encoding/json"
	"time"

	"plug-api/internal/device"
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

func DecodeState(s device.Snapshot) (State, error) {
	var state State
	state.ID, state.ReceivedAt = s.ID, s.ReceivedAt
	var err error
	if state.RelayOn, err = device.Field[bool](s.DPS, dpRelay); err != nil {
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
	state.Fault, err = device.Field[uint32](s.DPS, dpFault)
	return state, err
}

func scaled(dps map[string]json.RawMessage, id string, divisor float64) (*float64, error) {
	value, err := device.Field[float64](dps, id)
	if value != nil {
		*value /= divisor
	}
	return value, err
}
