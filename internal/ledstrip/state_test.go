package ledstrip

import (
	"encoding/json"
	"testing"

	"plug-api/internal/device"
)

func TestColorUpdates(t *testing.T) {
	snapshot := device.Snapshot{DPS: map[string]json.RawMessage{
		"20": json.RawMessage("true"),
		"21": json.RawMessage(`"colour"`),
		"24": json.RawMessage(`"007803e803e8"`),
		"26": json.RawMessage("0"),
	}}
	state, err := DecodeState(snapshot)
	if err != nil || state.Color == nil || *state.Color != "#00FF00" ||
		state.BrightnessPercent == nil || *state.BrightnessPercent != 100 {
		t.Fatalf("unexpected decoded light state: %v, %v", state, err)
	}
	orange, brightness, off := "#FF8000", 30, false
	tests := []struct {
		name  string
		patch Patch
		dp    string
		want  any
	}{
		{"color preserves brightness", Patch{Color: &orange}, "24", "001e03e803e8"},
		{"brightness preserves hue and saturation", Patch{BrightnessPercent: &brightness}, "24", "007803e8012c"},
		{"power only", Patch{On: &off}, "20", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, err := tt.patch.values(snapshot)
			if err != nil || values[tt.dp] != tt.want {
				t.Fatalf("values = %v, err = %v", values, err)
			}
			if tt.dp == "24" && (values["21"] != "colour" || len(values) != 2) {
				t.Fatalf("color update must write only mode and color: %v", values)
			}
			if tt.dp == "20" && len(values) != 1 {
				t.Fatalf("power update must write only power: %v", values)
			}
		})
	}
}
