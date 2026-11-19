package ledstrip

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"

	"plug-api/internal/device"
)

const (
	maxSegmentNumber = 255
	// The model allows 128 raw bytes, including a 12-byte paint header.
	maxSegmentSelection = 116
)

type Segment struct {
	Color             string `json:"color"`
	BrightnessPercent int    `json:"brightness_percent"`
}

func validateSegments(segments []*Segment) error {
	if len(segments) > maxSegmentNumber {
		return fmt.Errorf("segments must contain at most %d positions", maxSegmentNumber)
	}
	selected := 0
	for i, segment := range segments {
		if segment == nil {
			continue
		}
		selected++
		if !validRGB(segment.Color) || segment.Color == "#000000" {
			return fmt.Errorf("segment %d color must be a nonblack #RRGGBB hex value", i+1)
		}
		if segment.BrightnessPercent < 1 || segment.BrightnessPercent > 100 {
			return fmt.Errorf("segment %d brightness_percent must be an integer between 1 and 100", i+1)
		}
	}
	if selected == 0 || selected > maxSegmentSelection {
		return fmt.Errorf("provide between 1 and %d segment objects", maxSegmentSelection)
	}
	return nil
}

func segmentCommands(before device.Snapshot, state map[string]any, segments []*Segment) ([]map[string]any, error) {
	pixels, err := device.Field[int](before.DPS, dpPixels)
	if err != nil {
		return nil, err
	}
	configured, err := device.Field[int](before.DPS, dpConfiguredCount)
	if err != nil {
		return nil, err
	}
	if pixels == nil || *pixels < 1 {
		return nil, &device.Error{Code: "unsupported_setting", Result: "failed"}
	}
	count := *pixels
	if configured != nil {
		if *configured < 1 || *configured > count {
			return nil, &device.Error{Code: "invalid_device_response", Result: "failed"}
		}
		count = *configured
	}
	if count > maxSegmentNumber {
		return nil, &device.Error{Code: "unsupported_setting", Result: "failed"}
	}
	if len(segments) > count {
		return nil, &device.Error{Code: "segment_out_of_range", Result: "failed"}
	}

	on, err := device.Field[bool](before.DPS, dpPower)
	if err != nil {
		return nil, err
	}
	mode, err := device.Field[string](before.DPS, dpMode)
	if err != nil {
		return nil, err
	}
	powerWillBeOn := state[dpPower] == true || (on != nil && *on)
	modeWillBeColour := state[dpMode] == "colour" || (mode != nil && *mode == "colour")
	if !powerWillBeOn || !modeWillBeColour {
		return nil, &device.Error{Code: "light_not_ready_for_paint", Result: "failed"}
	}

	// One paint command can apply one HSV color to several indices.
	type selection struct {
		color   hsv
		indices []byte
	}
	groups := make([]selection, 0)
	groupIndex := make(map[hsv]int)
	for i, segment := range segments {
		if segment == nil {
			continue
		}
		color := hsv{value: segment.BrightnessPercent * 10}
		color.setRGB(segment.Color)
		index, exists := groupIndex[color]
		if !exists {
			index = len(groups)
			groupIndex[color] = index
			groups = append(groups, selection{color: color})
		}
		groups[index].indices = append(groups[index].indices, byte(i+1))
	}
	commands := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		commands = append(commands, map[string]any{dpPaint: encodePaint(count, group.color, group.indices)})
	}
	return commands, nil
}

func encodePaint(count int, color hsv, indices []byte) string {
	// Version, color mode, effect, segment count, pencil, HSV and selection.
	payload := []byte{0, 1, 0, byte(count), 1}
	payload = binary.BigEndian.AppendUint16(payload, uint16(color.hue))
	payload = binary.BigEndian.AppendUint16(payload, uint16(color.saturation))
	payload = binary.BigEndian.AppendUint16(payload, uint16(color.value))
	payload = append(payload, 0x80|byte(len(indices)))
	payload = append(payload, indices...)
	return base64.StdEncoding.EncodeToString(payload)
}
