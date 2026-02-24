package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

func TestCORSMiddlewareHandlesPreflight(t *testing.T) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(corsMiddleware())
	r.GET("/admin", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodOptions, "/admin", nil)
	req.Header.Set("Origin", "http://localhost:8081")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")
	req.RemoteAddr = "127.0.0.1:4000"

	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected preflight status %d, got %d", http.StatusNoContent, recorder.Code)
	}

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:8081" {
		t.Fatalf("expected Access-Control-Allow-Origin to echo request origin, got %q", got)
	}

	allowHeaders := recorder.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(strings.ToLower(allowHeaders), "authorization") {
		t.Fatalf("expected Access-Control-Allow-Headers to include Authorization, got %q", allowHeaders)
	}
}

func TestRateLimitMiddlewareSkipsOptions(t *testing.T) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(rateLimitMiddleware(newLimiterStore(rate.Limit(1), 1)))
	r.OPTIONS("/admin", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	r.GET("/admin", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	for idx := 0; idx < 4; idx++ {
		req := httptest.NewRequest(http.MethodOptions, "/admin", nil)
		req.RemoteAddr = "127.0.0.1:5000"

		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusNoContent {
			t.Fatalf("expected OPTIONS request %d to return 204, got %d", idx+1, recorder.Code)
		}
	}

	firstGet := httptest.NewRequest(http.MethodGet, "/admin", nil)
	firstGet.RemoteAddr = "127.0.0.1:5000"
	firstRecorder := httptest.NewRecorder()
	r.ServeHTTP(firstRecorder, firstGet)
	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("expected first GET to return 200, got %d", firstRecorder.Code)
	}

	secondGet := httptest.NewRequest(http.MethodGet, "/admin", nil)
	secondGet.RemoteAddr = "127.0.0.1:5000"
	secondRecorder := httptest.NewRecorder()
	r.ServeHTTP(secondRecorder, secondGet)
	if secondRecorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second GET to be rate-limited with 429, got %d", secondRecorder.Code)
	}
}
