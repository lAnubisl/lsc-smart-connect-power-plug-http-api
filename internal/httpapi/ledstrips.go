package httpapi

import (
	"net/http"

	"plug-api/internal/ledstrip"
)

func (a *API) stripState(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.strips.Read(r.Context(), r.PathValue("id"))
	if err != nil {
		deviceError(w, err)
		return
	}
	state, err := ledstrip.DecodeState(snapshot)
	if err != nil {
		deviceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (a *API) patchStripState(w http.ResponseWriter, r *http.Request) {
	var body ledstrip.Patch
	if !decodeBody(w, r, &body) {
		return
	}
	if err := body.Validate(); err != nil {
		apiError(w, http.StatusBadRequest, "invalid_light_settings", err.Error(), "failed")
		return
	}
	snapshot, err := a.strips.PatchState(r.Context(), r.PathValue("id"), body)
	if err != nil {
		deviceError(w, err)
		return
	}
	state, err := ledstrip.DecodeState(snapshot)
	if err != nil {
		apiError(w, http.StatusBadGateway, "invalid_device_response", "Write completed, but the light response could not be decoded", "unknown")
		return
	}
	result := "confirmed"
	if body.Segments != nil {
		result = "acknowledged"
	}
	writeJSON(w, http.StatusOK, struct {
		ledstrip.State
		Result string `json:"result"`
	}{State: state, Result: result})
}

func (a *API) startStripAnimation(w http.ResponseWriter, r *http.Request) {
	var body ledstrip.Animation
	if !decodeBody(w, r, &body) {
		return
	}
	if err := body.Validate(); err != nil {
		apiError(w, http.StatusBadRequest, "invalid_light_animation", err.Error(), "failed")
		return
	}
	snapshot, err := a.strips.StartAnimation(r.Context(), r.PathValue("id"), body)
	if err != nil {
		deviceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": snapshot.ID, "received_at": snapshot.ReceivedAt,
		"preset": body.Preset, "mode": "scene", "result": "acknowledged",
	})
}
