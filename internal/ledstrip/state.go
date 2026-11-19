package ledstrip

import (
	"time"

	"plug-api/internal/device"
)

type State struct {
	ID                string    `json:"id"`
	ReceivedAt        time.Time `json:"received_at"`
	On                *bool     `json:"on"`
	Mode              *string   `json:"mode"`
	Color             *string   `json:"color"`
	BrightnessPercent *float64  `json:"brightness_percent"`
	CountdownSeconds  *int      `json:"countdown_seconds"`
}

func DecodeState(s device.Snapshot) (State, error) {
	state := State{ID: s.ID, ReceivedAt: s.ReceivedAt}
	var err error
	if state.On, err = device.Field[bool](s.DPS, dpPower); err != nil {
		return state, err
	}
	if state.Mode, err = device.Field[string](s.DPS, dpMode); err != nil {
		return state, err
	}
	if state.CountdownSeconds, err = device.Field[int](s.DPS, dpCountdown); err != nil {
		return state, err
	}
	color, err := decodeHSV(s)
	if err != nil || color == nil {
		return state, err
	}
	rgb := color.rgb()
	brightness := float64(color.value) / 10
	state.Color, state.BrightnessPercent = &rgb, &brightness
	return state, nil
}
