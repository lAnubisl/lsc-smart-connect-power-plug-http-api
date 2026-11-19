package ledstrip

import (
	"fmt"

	"plug-api/internal/device"
)

// Scene packets from the Tuya Local profile for LSC Smart Connect Neon Striplight.
// https://github.com/make-all/tuya-local/blob/main/custom_components/tuya_local/devices/lsc_neon_ledstrip.yaml
var animationPresets = map[string]string{
	"iceland_blue":     "ARUKUlLgAABkAMFhALQwALVSAMRj",
	"glacier_express":  "ARYKZGRgAABkAJJfAMZg",
	"sea_of_clouds":    "ARcDXl5gAABkADgvAB5cANVFARpk",
	"fireworks_at_sea": "ARgCZGTgAABkALI5AQpkAS1kAT9k",
	"hut_in_the_snow":  "ARkKVFRgAABkALEsAMBk",
	"firefly_night":    "ARoDS0vgAABkAOA5AQlT",
	"northland":        "ARsDX19gAABkAK45AMRdAPlk",
	"grassland":        "ARwKWlrgAABSAJ1kAI5k",
	"northern_lights":  "AR0DUlLgAABkAK5kAKZkAMFkAMxk",
	"late_autumn":      "AR4KUlLgAABkABlkACJeACxbABRkAAxk",
	"dream_meteor":     "AUcFTU0AAABkAQNFAMFD",
	"early_spring":     "AUgGMjIAAABkAU5BAB9J",
	"spring_outing":    "AUkHDg4AAABkANo3AVJBAFw3",
	"night_service":    "AUoIMjIAAABkAPdQAClPAQ04AKMn",
	"wind_chime":       "AUsJMjIAAABkAQNFAEE6ACVLAF5C",
	"city_lights":      "AUwMMjIAAABkANhNAMFDAQNFAFw3",
	"colour_marbles":   "AU0NMjIAAABkAChkAF5CAMFkAP9Q",
	"summer_train":     "AU4OMjIAAABkAD5fAL5c",
	"christmas_eve":    "AU8PGRkAAABkALxkAC1OAABkAGQ8",
	"dream_sea":        "AVAQMjIAAABkAOZHAGQ8ARlNALg5",
	"game":             "AR8CX19gAABkARBkANJkAK1kAItk",
	"holiday":          "ASAKVVVgAABkAMJYAT4zAP9GAR1k",
	"work":             "ASEDPDxgAABkAL8YAQQX",
	"party":            "ASIEZGRgAABkANdcALxTADceACw/AWE/",
	"trend":            "ASMCZGRgAABkAQhLALEvAM1X",
	"sports":           "ASQKS0tgAABkALwmANZVARhkAPlN",
	"meditation":       "ASUDQ0NgAABkALc1AJtUAM1h",
	"dating":           "ASYBWVngAABkARlHAUk9AM1hACZk",
	"christmas":        "ASkCYWHgAABkAAtkANlkACtkAJFkALlk",
	"valentines_day":   "ASoBZGRgAABkARVkAQVkAUVkAS9k",
	"halloween":        "ASsDWlrgAABkAABXARZkANpkALNkAJVk",
	"thanksgiving_day": "ASwKSEhgAABkAD1kAQxbALpJABdh",
	"forest_day":       "AS0CWVlgAABkAJxjALxiAHtg",
	"mothers_day":      "AS4DWlpgAABkAT42AQxWAR8j",
	"fathers_day":      "AS8CZGTgAABkANxCALZKAOFN",
	"football_day":     "ATACXl5gAABkAABkAHhkALtk",
	"summer_idyll":     "ATMDUlJgAABkAIhQANI5APsn",
	"dream_of_the_sea": "ATQDXV1gAABkAPc2ATUrAMY0AJEp",
	"love_and_dream":   "ATUDUlJgAABNARJiATBd",
	"spring_fishing":   "ATYCSUlgAABkAGY8ADxJAB5k",
	"neon_world":       "ATcKWlpgAABkADNYABhkAQBFAONeAKww",
	"dreamland":        "ATgCV1fgAABkAQxkARpBAUdZABVkADw4",
	"summer_wind":      "ATkDSEjgAABkAFlkALNH",
	"planet_journey":   "AToCXV3gAABNALReARxkAOhJAMZf",
}

type Animation struct {
	Preset string `json:"preset"`
	On     *bool  `json:"on"`
}

func (a Animation) Validate() error {
	if _, ok := animationPresets[a.Preset]; !ok {
		return fmt.Errorf("provide a supported animation preset")
	}
	if a.On != nil && !*a.On {
		return fmt.Errorf("on must be true when supplied")
	}
	return nil
}

func (a Animation) plan(before device.Snapshot) (device.WritePlan, error) {
	state := map[string]any{dpMode: "scene"}
	if a.On != nil {
		state[dpPower] = true
	} else {
		on, err := device.Field[bool](before.DPS, dpPower)
		if err != nil {
			return device.WritePlan{}, err
		}
		if on == nil || !*on {
			return device.WritePlan{}, &device.Error{Code: "light_not_on", Result: "failed"}
		}
	}
	// CONTROL carries the scene and mode together. STATUS reports the mode.
	return device.WritePlan{
		State:       state,
		ControlOnly: map[string]any{dpScene: animationPresets[a.Preset]},
	}, nil
}
