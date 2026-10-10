package tracer

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTracerStartAndMiddleware(t *testing.T) {
	shutdown := Start(func(c *Config) {
		c.ServiceName = "unit-test-service"
		c.Endpoint = "http://127.0.0.1:4318"
	})
	defer shutdown()

	handler := Middleware("unit-test-service")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("GET", "/test-endpoint", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
}
