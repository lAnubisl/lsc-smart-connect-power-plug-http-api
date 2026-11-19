package device

import (
	"encoding/json"
	"testing"
)

func TestVerificationDatapointRules(t *testing.T) {
	rules := Verification{CountdownDPS: "timer", CaseInsensitiveDPS: "color"}
	tests := []struct {
		name     string
		dp       string
		actual   string
		expected any
		want     bool
	}{
		{"timer can lose two seconds", "timer", `8`, 10, true},
		{"timer cannot lose three seconds", "timer", `7`, 10, false},
		{"running timer cannot read zero", "timer", `0`, 1, false},
		{"disabled timer reads zero", "timer", `0`, 0, true},
		{"other numbers require equality", "level", `8`, 10, false},
		{"color ignores case", "color", `"00AF"`, "00af", true},
		{"other strings require matching case", "mode", `"COLOUR"`, "colour", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dps := map[string]json.RawMessage{tt.dp: json.RawMessage(tt.actual)}
			values := map[string]any{tt.dp: tt.expected}
			if got := rules.matches(dps, values); got != tt.want {
				t.Fatalf("matches() = %t, want %t", got, tt.want)
			}
		})
	}
}
