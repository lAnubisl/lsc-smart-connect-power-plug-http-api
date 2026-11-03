// Package httpapi exposes the device operations and embedded Swagger UI.
package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"plug-api/internal/config"
	"plug-api/internal/logging"
	"plug-api/internal/plug"
)

//go:embed docs/* openapi.json
var assets embed.FS

type API struct {
	plugs   *plug.Service
	logger  *slog.Logger
	config  config.HTTP
	timeout time.Duration
}

func New(cfg config.HTTP, plugs *plug.Service, logger *slog.Logger) http.Handler {
	a := &API{plugs: plugs, logger: logger, config: cfg, timeout: time.Duration(cfg.RequestTimeoutSeconds) * time.Second}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /plugs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"plugs": a.plugs.IDs()})
	})
	mux.HandleFunc("GET /plugs/{id}/state", a.state)
	mux.HandleFunc("GET /plugs/{id}/settings", a.settings)
	mux.HandleFunc("GET /plugs/{id}/dps", a.dps)
	mux.HandleFunc("PUT /plugs/{id}/relay", a.relay)
	mux.HandleFunc("PATCH /plugs/{id}/settings", a.patchSettings)
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, r *http.Request) {
		data, _ := assets.ReadFile("openapi.json")
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})
	docs, _ := fs.Sub(assets, "docs")
	mux.Handle("GET /docs/", http.StripPrefix("/docs/", http.FileServer(http.FS(docs))))
	return a.middleware(mux)
}

func (a *API) state(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.plugs.Read(r.Context(), r.PathValue("id"))
	if err != nil {
		deviceError(w, err)
		return
	}
	state, err := snapshot.State()
	if err != nil {
		deviceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (a *API) settings(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.plugs.Read(r.Context(), r.PathValue("id"))
	if err != nil {
		deviceError(w, err)
		return
	}
	settings, err := snapshot.Settings()
	if err != nil {
		deviceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (a *API) dps(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.plugs.Read(r.Context(), r.PathValue("id"))
	if err != nil {
		deviceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (a *API) relay(w http.ResponseWriter, r *http.Request) {
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

func (a *API) patchSettings(w http.ResponseWriter, r *http.Request) {
	var body plug.Settings
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
	settings, err := snapshot.Settings()
	if err != nil {
		apiError(w, 502, "invalid_device_response", "Write completed, but the settings response could not be decoded", "unknown")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": snapshot.ID, "result": "confirmed", "received_at": snapshot.ReceivedAt, "settings": settings.Settings,
	})
}

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
	var failure *plug.Error
	if !errors.As(err, &failure) {
		apiError(w, 500, "internal_error", "Request could not be completed", "unknown")
		return
	}
	status := http.StatusBadGateway
	switch failure.Code {
	case "plug_not_found":
		status = http.StatusNotFound
	case "plug_busy":
		status = http.StatusServiceUnavailable
		w.Header().Set("Retry-After", "1")
	case "device_timeout":
		status = http.StatusGatewayTimeout
	case "request_canceled":
		status = http.StatusRequestTimeout
	case "unsupported_setting":
		status = http.StatusUnprocessableEntity
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

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (a *API) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		idBytes := make([]byte, 12)
		if _, err := rand.Read(idBytes); err != nil {
			apiError(w, 500, "internal_error", "Could not create request ID", "failed")
			return
		}
		requestID := hex.EncodeToString(idBytes)
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		rw := &responseWriter{ResponseWriter: w}
		ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
		defer cancel()
		r = r.WithContext(logging.WithRequestID(ctx, requestID))
		logger := a.logger.With("request_id", requestID)
		defer func() {
			if recover() != nil {
				logger.Error("request handler panicked")
				if rw.status == 0 {
					apiError(rw, 500, "internal_error", "Request could not be completed", "unknown")
				}
			}
			level := slog.LevelInfo
			if rw.status >= 400 {
				level = slog.LevelWarn
			}
			// Log the route template, never headers, bodies, secrets, or query strings.
			logger.Log(ctx, level, "HTTP request completed", "method", r.Method, "route", r.Pattern, "status", rw.status, "duration_ms", time.Since(started).Milliseconds())
		}()
		if (r.URL.Path == "/plugs" || strings.HasPrefix(r.URL.Path, "/plugs/")) && a.config.Token != "" {
			provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || subtle.ConstantTimeCompare([]byte(provided), []byte(a.config.Token)) != 1 {
				w.Header().Set("WWW-Authenticate", "Bearer")
				apiError(rw, 401, "unauthorized", "A valid bearer token is required", "failed")
				return
			}
		}
		next.ServeHTTP(rw, r)
	})
}
