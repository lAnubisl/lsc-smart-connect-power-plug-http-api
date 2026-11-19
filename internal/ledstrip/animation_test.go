package ledstrip

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"plug-api/internal/device"
)

func TestAnimationWritePlan(t *testing.T) {
	before := device.Snapshot{DPS: map[string]json.RawMessage{
		"20": json.RawMessage("true"),
		"21": json.RawMessage(`"colour"`),
	}}
	animation := Animation{Preset: "northern_lights"}
	plan, err := animation.plan(before)
	if err != nil || len(plan.State) != 1 || plan.State["21"] != "scene" ||
		plan.ControlOnly[dpScene] != "AR0DUlLgAABkAK5kAKZkAMFkAMxk" {
		t.Fatalf("unexpected scene plan: %+v, err = %v", plan, err)
	}
	before.DPS["20"] = json.RawMessage("false")
	_, err = animation.plan(before)
	var failure *device.Error
	if !errors.As(err, &failure) || failure.Code != "light_not_on" {
		t.Fatalf("expected an off-strip error, got %v", err)
	}
	on := true
	animation.On = &on
	plan, err = animation.plan(before)
	if err != nil || plan.State["20"] != true || plan.State["21"] != "scene" {
		t.Fatalf("unexpected power and scene plan: %+v, err = %v", plan, err)
	}
}

func TestAnimationPresetPackets(t *testing.T) {
	for preset, encoded := range animationPresets {
		payload, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(payload) < 12 || len(payload) > 128 || (len(payload)-9)%3 != 0 || payload[0] != 1 {
			t.Errorf("%s: invalid scene packet: %x, err = %v", preset, payload, err)
		}
	}
}
