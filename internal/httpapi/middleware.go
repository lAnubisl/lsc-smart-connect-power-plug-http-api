package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"plug-api/internal/logging"
)

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
		path := r.URL.Path
		isDeviceRoute := path == "/plugs" || strings.HasPrefix(path, "/plugs/") ||
			path == "/lights" || strings.HasPrefix(path, "/lights/")
		if isDeviceRoute && a.config.Token != "" {
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
