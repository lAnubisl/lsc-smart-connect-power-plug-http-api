package httpapi

import (
	"net/http"

	"plug-api/internal/smartplug"
)

func (a *API) plugState(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.plugs.Read(r.Context(), r.PathValue("id"))
	if err != nil {
		deviceError(w, err)
		return
	}
	state, err := smartplug.DecodeState(snapshot)
	if err != nil {
		deviceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (a *API) plugSettings(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.plugs.Read(r.Context(), r.PathValue("id"))
	if err != nil {
		deviceError(w, err)
		return
	}
	settings, err := smartplug.DecodeSettings(snapshot)
	if err != nil {
		deviceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (a *API) setPlugRelay(w http.ResponseWriter, r *http.Request) {
	var body struct {
		On *bool `json:"on"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.On == nil {
		apiError(w, 400, "invalid_body", "on is required", "failed")
		return
	}
	snapshot, err := a.plugs.SetRelay(r.Context(), r.PathValue("id"), *body.On)
	if err != nil {
		deviceError(w, err)
		return
	}
	// The service already verified the relay data point against the requested value.
	writeJSON(w, http.StatusOK, map[string]any{
		"id": snapshot.ID, "result": "confirmed", "received_at": snapshot.ReceivedAt, "relay_on": *body.On,
	})
}

func (a *API) patchPlugSettings(w http.ResponseWriter, r *http.Request) {
	var body smartplug.Settings
	if !decodeBody(w, r, &body) {
		return
	}
	if _, err := body.Values(); err != nil {
		apiError(w, 400, "invalid_settings", err.Error(), "failed")
		return
	}
	snapshot, err := a.plugs.SetSettings(r.Context(), r.PathValue("id"), body)
	if err != nil {
		deviceError(w, err)
		return
	}
	settings, err := smartplug.DecodeSettings(snapshot)
	if err != nil {
		apiError(w, 502, "invalid_device_response", "Write completed, but the settings response could not be decoded", "unknown")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": snapshot.ID, "result": "confirmed", "received_at": snapshot.ReceivedAt, "settings": settings.Settings,
	})
}
