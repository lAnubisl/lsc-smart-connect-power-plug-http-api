package device

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"plug-api/internal/tuyalan"
)

// Verification defines how a device reports written values in STATUS.
type Verification struct {
	CountdownDPS       string
	CaseInsensitiveDPS string
	RetryDelay         time.Duration
}

func (s *Service) verifyState(ctx context.Context, client *tuyalan.Client, id string, values map[string]any) (Snapshot, error) {
	snapshot, err := readSnapshot(client, id)
	if err != nil || s.verification.matches(snapshot.DPS, values) {
		return snapshot, err
	}
	if s.verification.RetryDelay > 0 {
		// Some devices acknowledge a write before STATUS reflects the change.
		// Recheck once while holding the device gate without resending CONTROL.
		timer := time.NewTimer(s.verification.RetryDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return snapshot, contextError(ctx)
		case <-timer.C:
		}
		recheck, err := readSnapshot(client, id)
		if err != nil {
			return snapshot, err
		}
		snapshot = recheck
		if s.verification.matches(snapshot.DPS, values) {
			return snapshot, nil
		}
	}
	return snapshot, &Error{Code: "verification_failed", Result: "unknown"}
}

func (v Verification) matches(dps map[string]json.RawMessage, values map[string]any) bool {
	for dp, expected := range values {
		if !v.confirmed(dps[dp], expected, dp) {
			return false
		}
	}
	return true
}

func sameType(raw json.RawMessage, expected any) bool {
	var actual any
	if json.Unmarshal(raw, &actual) != nil {
		return false
	}
	switch expected.(type) {
	case bool:
		_, ok := actual.(bool)
		return ok
	case string:
		_, ok := actual.(string)
		return ok
	case int:
		_, ok := actual.(float64)
		return ok
	}
	return false
}

func (verification Verification) confirmed(raw json.RawMessage, expected any, dp string) bool {
	var actual any
	if json.Unmarshal(raw, &actual) != nil {
		return false
	}
	switch v := expected.(type) {
	case bool:
		return actual == v
	case string:
		if dp == verification.CaseInsensitiveDPS {
			got, ok := actual.(string)
			return ok && strings.EqualFold(got, v)
		}
		return actual == v
	case int:
		number, ok := actual.(float64)
		if !ok {
			return false
		}
		// A countdown can decrease between writing it and reading it back.
		if dp == verification.CountdownDPS && v > 0 {
			return number > 0 && number <= float64(v) && number >= float64(v)-2
		}
		return number == float64(v)
	}
	return false
}
