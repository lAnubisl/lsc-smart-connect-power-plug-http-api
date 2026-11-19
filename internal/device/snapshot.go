package device

import (
	"encoding/json"
	"time"

	"plug-api/internal/tuyalan"
)

type Snapshot struct {
	ID         string                     `json:"id"`
	ReceivedAt time.Time                  `json:"received_at"`
	DPS        map[string]json.RawMessage `json:"dps"`
}

func readSnapshot(client *tuyalan.Client, id string) (Snapshot, error) {
	dps, err := client.Status()
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{ID: id, ReceivedAt: time.Now().UTC(), DPS: dps}, nil
}

// Field returns nil for a missing or null datapoint.
func Field[T any](dps map[string]json.RawMessage, id string) (*T, error) {
	raw, exists := dps[id]
	if !exists || string(raw) == "null" {
		return nil, nil
	}
	var value T
	if json.Unmarshal(raw, &value) != nil {
		return nil, &Error{Code: "invalid_device_response", Result: "failed"}
	}
	return &value, nil
}
