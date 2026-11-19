package httpapi

import (
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"plug-api/internal/config"
	"plug-api/internal/ledstrip"
	"plug-api/internal/smartplug"
)

func TestDeviceRoutesAuthAndIsolation(t *testing.T) {
	plugs := smartplug.New([]config.Device{{ID: "socket"}}, slog.Default())
	strips := ledstrip.New([]config.Device{{ID: "neon"}}, slog.Default())
	api := New(config.HTTP{RequestTimeoutSeconds: 1, Token: "test-token"}, plugs, strips, slog.Default())
	tests := []struct {
		method, path, token, body string
		status                    int
		response                  string
	}{
		{"GET", "/lights", "", "", 401, "unauthorized"},
		{"POST", "/lights/neon/animation", "", `{"preset":"northern_lights"}`, 401, "unauthorized"},
		{"POST", "/lights/neon/animation", "test-token", `{"preset":"unknown"}`, 400, "invalid_light_animation"},
		{"POST", "/lights/neon/animation", "test-token", `{"preset":"northern_lights","on":false}`, 400, "invalid_light_animation"},
		{"POST", "/lights/socket/animation", "test-token", `{"preset":"dream_meteor","on":true}`, 404, "light_not_found"},
		{"PATCH", "/lights/neon/state", "test-token", `{"animation":{"preset":"dream_meteor"}}`, 400, "invalid_body"},
		{"PATCH", "/lights/neon/segments", "test-token", `{"segments":[5],"color":"#FF0000"}`, 404, "404 page not found"},
		{"PATCH", "/lights/socket/state", "test-token", `{"segments":[null,{"color":"#FF0000","brightness_percent":30}]}`, 404, "light_not_found"},
		{"PATCH", "/lights/neon/state", "test-token", `{"segments":[{"color":"#FF0000"}]}`, 400, "segment 1 brightness_percent"},
		{"PATCH", "/lights/neon/state", "test-token", `{"segments":[null]}`, 400, "segment objects"},
		{"PATCH", "/lights/neon/state", "test-token", `{"on":false,"segments":[{"color":"#FF0000","brightness_percent":30}]}`, 400, "segments cannot be combined"},
		{"PATCH", "/lights/neon/state", "test-token", `{"segments":[{"color":"#FF0000","brightness_percent":30,"id":5}]}`, 400, "invalid_body"},
		{"PATCH", "/lights/neon/state", "", `{"on":false}`, 401, "unauthorized"},
		{"GET", "/lights", "test-token", "", 200, `{"lights":["neon"]}`},
		{"GET", "/plugs", "test-token", "", 200, `{"plugs":["socket"]}`},
		{"PUT", "/plugs/neon/relay", "test-token", `{"on":false}`, 404, "plug_not_found"},
		{"PATCH", "/lights/socket/state", "test-token", `{"on":false}`, 404, "light_not_found"},
		{"PATCH", "/lights/neon/state", "test-token", `{"brightness_percent":101}`, 400, "invalid_light_settings"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path+" "+tt.response, func(t *testing.T) {
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			if tt.token != "" {
				request.Header.Set("Authorization", "Bearer "+tt.token)
			}
			recorder := httptest.NewRecorder()
			api.ServeHTTP(recorder, request)
			if recorder.Code != tt.status || !strings.Contains(recorder.Body.String(), tt.response) {
				t.Fatalf("HTTP %d %s, want HTTP %d containing %q", recorder.Code, recorder.Body.String(), tt.status, tt.response)
			}
		})
	}
}
