package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"plug-api/internal/device"
)

func decodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		apiError(w, 415, "unsupported_media_type", "Content-Type must be application/json", "failed")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			apiError(w, 413, "body_too_large", "Request body exceeds 4096 bytes", "failed")
		} else {
			apiError(w, 400, "invalid_body", "Could not read request body", "failed")
		}
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || len(fields) == 0 {
		apiError(w, 400, "invalid_body", "Provide a nonempty JSON object", "failed")
		return false
	}
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			apiError(w, 400, "invalid_body", "Null settings are not accepted; omit unchanged fields", "failed")
			return false
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		apiError(w, 400, "invalid_body", "Invalid field name or field type", "failed")
		return false
	}
	return true
}

func deviceError(w http.ResponseWriter, err error) {
	var failure *device.Error
	if !errors.As(err, &failure) {
		apiError(w, 500, "internal_error", "Request could not be completed", "unknown")
		return
	}
	status := http.StatusBadGateway
	switch failure.Code {
	case "plug_not_found", "light_not_found":
		status = http.StatusNotFound
	case "plug_busy", "light_busy":
		status = http.StatusServiceUnavailable
		w.Header().Set("Retry-After", "1")
	case "device_timeout":
		status = http.StatusGatewayTimeout
	case "request_canceled":
		status = http.StatusRequestTimeout
	case "unsupported_setting", "segment_out_of_range":
		status = http.StatusUnprocessableEntity
	case "light_not_ready_for_paint", "light_not_on":
		status = http.StatusConflict
	}
	apiError(w, status, failure.Code, "Device operation did not complete successfully", failure.Result)
}

func apiError(w http.ResponseWriter, status int, code, message, result string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message, "result": result}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
