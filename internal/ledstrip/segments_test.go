package ledstrip

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	"plug-api/internal/device"
)

func TestSegmentCommands(t *testing.T) {
	red := &Segment{Color: "#FF0000", BrightnessPercent: 30}
	blue := &Segment{Color: "#0000FF", BrightnessPercent: 50}
	patch := Patch{Segments: []*Segment{nil, blue, nil, nil, red, nil, red}}
	before := device.Snapshot{DPS: map[string]json.RawMessage{
		"20": json.RawMessage("true"), "21": json.RawMessage(`"colour"`),
		"24": json.RawMessage(`"007803e803e8"`),
		"47": json.RawMessage("36"), "53": json.RawMessage("36"),
	}}
	plan, err := patch.plan(before)
	if err != nil || len(plan.State) != 0 || len(plan.Commands) != 2 {
		t.Fatalf("invalid write plan: %+v, err = %v", plan, err)
	}
	for i, want := range []string{"000100240100f003e801f48102", "0001002401000003e8012c820507"} {
		encoded, ok := plan.Commands[i][dpPaint].(string)
		if !ok {
			t.Fatalf("command %d is missing its paint payload: %v", i, plan.Commands[i])
		}
		payload, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || hex.EncodeToString(payload) != want {
			t.Fatalf("paint %d = %x, err = %v", i, payload, err)
		}
	}
	before.DPS["53"] = json.RawMessage("6")
	_, err = patch.plan(before)
	var failure *device.Error
	if !errors.As(err, &failure) || failure.Code != "segment_out_of_range" {
		t.Fatalf("expected range error, got %v", err)
	}
	before.DPS["53"] = json.RawMessage("36")
	before.DPS["20"] = json.RawMessage("false")
	before.DPS["21"] = json.RawMessage(`"scene"`)
	_, err = patch.plan(before)
	if !errors.As(err, &failure) || failure.Code != "light_not_ready_for_paint" {
		t.Fatalf("expected power and mode preflight error, got %v", err)
	}
	on, green := true, "#00FF00"
	patch.On, patch.Color = &on, &green
	plan, err = patch.plan(before)
	if err != nil || plan.State["20"] != true || plan.State["21"] != "colour" || len(plan.Commands) != 2 {
		t.Fatalf("combined update plan: %+v, err = %v", plan, err)
	}
}
