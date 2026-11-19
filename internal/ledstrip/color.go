package ledstrip

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"

	"plug-api/internal/device"
)

type hsv struct {
	hue        int // degrees, 0–360
	saturation int // 0–1000
	value      int // 0–1000
}

func decodeHSV(s device.Snapshot) (*hsv, error) {
	raw, err := device.Field[string](s.DPS, dpColor)
	if err != nil || raw == nil {
		return nil, err
	}
	data, err := hex.DecodeString(*raw)
	if err != nil || len(data) != 6 {
		return nil, &device.Error{Code: "invalid_device_response", Result: "failed"}
	}
	color := hsv{
		hue:        int(binary.BigEndian.Uint16(data[0:2])),
		saturation: int(binary.BigEndian.Uint16(data[2:4])),
		value:      int(binary.BigEndian.Uint16(data[4:6])),
	}
	if color.hue > 360 || color.saturation > 1000 || color.value > 1000 {
		return nil, &device.Error{Code: "invalid_device_response", Result: "failed"}
	}
	return &color, nil
}

func validRGB(color string) bool {
	if len(color) != 7 || color[0] != '#' {
		return false
	}
	_, err := hex.DecodeString(color[1:])
	return err == nil
}

func (c *hsv) setRGB(color string) {
	// The input was validated. Use RGB for hue and saturation, keeping value separate.
	rgb, _ := hex.DecodeString(color[1:])
	r, g, b := float64(rgb[0]), float64(rgb[1]), float64(rgb[2])
	peak := max(r, g, b)
	delta := peak - min(r, g, b)
	c.saturation = int(math.Round(1000 * delta / peak))
	if delta == 0 {
		c.hue = 0
		return
	}
	var hue float64
	switch peak {
	case r:
		hue = (g - b) / delta
	case g:
		hue = (b-r)/delta + 2
	case b:
		hue = (r-g)/delta + 4
	}
	c.hue = int(math.Round(60*hue)+360) % 360
}

func (c hsv) rgb() string {
	hue := float64(c.hue%360) / 60
	saturation := float64(c.saturation) / 1000
	secondary := saturation * (1 - math.Abs(math.Mod(hue, 2)-1))
	var channels [3]float64
	switch int(hue) {
	case 0:
		channels = [3]float64{saturation, secondary, 0}
	case 1:
		channels = [3]float64{secondary, saturation, 0}
	case 2:
		channels = [3]float64{0, saturation, secondary}
	case 3:
		channels = [3]float64{0, secondary, saturation}
	case 4:
		channels = [3]float64{secondary, 0, saturation}
	case 5:
		channels = [3]float64{saturation, 0, secondary}
	}
	for i, channel := range channels {
		channels[i] = math.Round(255 * (channel + 1 - saturation))
	}
	return fmt.Sprintf("#%02X%02X%02X", int(channels[0]), int(channels[1]), int(channels[2]))
}
