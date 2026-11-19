// Package httpapi exposes the device operations and embedded Swagger UI.
package httpapi

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"plug-api/internal/config"
	"plug-api/internal/device"
	"plug-api/internal/ledstrip"
	"plug-api/internal/smartplug"
)

//go:embed docs/* openapi.json
var assets embed.FS

type API struct {
	plugs   *smartplug.Service
	strips  *ledstrip.Service
	logger  *slog.Logger
	config  config.HTTP
	timeout time.Duration
}

func New(cfg config.HTTP, plugs *smartplug.Service, strips *ledstrip.Service, logger *slog.Logger) http.Handler {
	a := &API{plugs: plugs, strips: strips, logger: logger, config: cfg, timeout: time.Duration(cfg.RequestTimeoutSeconds) * time.Second}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /plugs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"plugs": a.plugs.IDs()})
	})
	mux.HandleFunc("GET /plugs/{id}/state", a.plugState)
	mux.HandleFunc("GET /plugs/{id}/settings", a.plugSettings)
	mux.HandleFunc("GET /plugs/{id}/dps", dps(a.plugs.Read))
	mux.HandleFunc("PUT /plugs/{id}/relay", a.setPlugRelay)
	mux.HandleFunc("PATCH /plugs/{id}/settings", a.patchPlugSettings)
	mux.HandleFunc("GET /lights", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"lights": a.strips.IDs()})
	})
	mux.HandleFunc("GET /lights/{id}/state", a.stripState)
	mux.HandleFunc("PATCH /lights/{id}/state", a.patchStripState)
	mux.HandleFunc("POST /lights/{id}/animation", a.startStripAnimation)
	mux.HandleFunc("GET /lights/{id}/dps", dps(a.strips.Read))
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, r *http.Request) {
		data, _ := assets.ReadFile("openapi.json")
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})
	docs, _ := fs.Sub(assets, "docs")
	mux.Handle("GET /docs/", http.StripPrefix("/docs/", http.FileServer(http.FS(docs))))
	return a.middleware(mux)
}

func dps(read func(context.Context, string) (device.Snapshot, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := read(r.Context(), r.PathValue("id"))
		if err != nil {
			deviceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
	}
}
