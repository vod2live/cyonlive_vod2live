package main

import (
	"net/http/httptest"
	"testing"
)

func TestParsePagination(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantLimit  int
		wantOffset int
		wantError  bool
	}{
		{name: "defaults", wantLimit: defaultPageSize},
		{name: "explicit page", query: "?limit=25&offset=50", wantLimit: 25, wantOffset: 50},
		{name: "maximum page", query: "?limit=200", wantLimit: 200},
		{name: "zero limit", query: "?limit=0", wantError: true},
		{name: "limit above maximum", query: "?limit=201", wantError: true},
		{name: "non numeric limit", query: "?limit=abc", wantError: true},
		{name: "negative offset", query: "?offset=-1", wantError: true},
		{name: "non numeric offset", query: "?offset=abc", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/v1/channels"+tt.query, nil)
			limit, offset, err := parsePagination(req)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected invalid pagination to return an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePagination returned unexpected error: %v", err)
			}
			if limit != tt.wantLimit || offset != tt.wantOffset {
				t.Fatalf("got limit=%d offset=%d, want limit=%d offset=%d", limit, offset, tt.wantLimit, tt.wantOffset)
			}
		})
	}
}
