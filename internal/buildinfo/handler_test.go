package buildinfo

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler(t *testing.T) {
	rr := httptest.NewRecorder()
	Handler(rr, httptest.NewRequest(http.MethodGet, "/version", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "version") {
		t.Fatalf("unexpected response: %s", rr.Body.String())
	}
}
