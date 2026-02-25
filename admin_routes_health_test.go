package main

import (
	"net/http"
	"testing"
)

func TestHealthHTTPStatusCode(t *testing.T) {
	testCases := []struct {
		name     string
		status   string
		expected int
	}{
		{
			name:     "ok status returns 200",
			status:   healthStatusOK,
			expected: http.StatusOK,
		},
		{
			name:     "degraded status returns 503",
			status:   healthStatusDegraded,
			expected: http.StatusServiceUnavailable,
		},
		{
			name:     "down status returns 503",
			status:   healthStatusDown,
			expected: http.StatusServiceUnavailable,
		},
		{
			name:     "unknown status returns 503",
			status:   "unexpected",
			expected: http.StatusServiceUnavailable,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := healthHTTPStatusCode(tc.status)
			if got != tc.expected {
				t.Fatalf("expected %d, got %d", tc.expected, got)
			}
		})
	}
}
