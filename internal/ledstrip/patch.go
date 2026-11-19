package ledstrip

import (
	"fmt"

	"plug-api/internal/device"
)

type Patch struct {
	On                *bool      `json:"on"`
	Color             *string    `json:"color"`
	BrightnessPercent *int       `json:"brightness_percent"`
	CountdownSeconds  *int       `json:"countdown_seconds"`
	Segments          []*Segment `json:"segments"`
}

func (p Patch) Validate() error {
	if p.On == nil && p.Color == nil && p.BrightnessPercent == nil && p.CountdownSeconds == nil && p.Segments == nil {
		return fmt.Errorf("provide at least one light setting")
	}
	if p.Color != nil {
		color := *p.Color
		if !validRGB(color) {
			return fmt.Errorf("color must be a #RRGGBB hex value")
		}
		if color == "#000000" {
			return fmt.Errorf("use on=false to turn the light off")
		}
	}
	if p.BrightnessPercent != nil && (*p.BrightnessPercent < 1 || *p.BrightnessPercent > 100) {
		return fmt.Errorf("brightness_percent must be an integer between 1 and 100")
	}
	if p.CountdownSeconds != nil && (*p.CountdownSeconds < 0 || *p.CountdownSeconds > 86400) {
		return fmt.Errorf("countdown_seconds must be between 0 and 86400")
	}
	if p.Segments != nil {
		if p.On != nil && !*p.On {
			return fmt.Errorf("segments cannot be combined with on=false")
		}
		return validateSegments(p.Segments)
	}
	return nil
}

func (p Patch) plan(before device.Snapshot) (device.WritePlan, error) {
	state, err := p.values(before)
	if err != nil || p.Segments == nil {
		return device.WritePlan{State: state}, err
	}
	commands, err := segmentCommands(before, state, p.Segments)
	return device.WritePlan{State: state, Commands: commands}, err
}

func (p Patch) values(before device.Snapshot) (map[string]any, error) {
	values := make(map[string]any)
	if p.On != nil {
		values[dpPower] = *p.On
	}
	if p.CountdownSeconds != nil {
		values[dpCountdown] = *p.CountdownSeconds
	}
	if p.Color == nil && p.BrightnessPercent == nil {
		return values, nil
	}

	// Derive partial updates from the preflight status while holding the device gate.
	color, err := decodeHSV(before)
	if err != nil {
		return nil, err
	}
	if color == nil {
		return nil, &device.Error{Code: "unsupported_setting", Result: "failed"}
	}
	if p.Color != nil {
		color.setRGB(*p.Color)
	}
	if p.BrightnessPercent != nil {
		color.value = *p.BrightnessPercent * 10
	}
	values[dpMode] = "colour"
	values[dpColor] = fmt.Sprintf("%04x%04x%04x", color.hue, color.saturation, color.value)
	return values, nil
}
