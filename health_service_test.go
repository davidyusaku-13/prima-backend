package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHealthServiceSnapshot_OK(t *testing.T) {
	startedAt := time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC)
	checkedAt := time.Date(2026, 2, 24, 12, 0, 42, 0, time.UTC)

	svc := NewHealthService(HealthServiceDeps{
		StartedAt: startedAt,
		Now:       func() time.Time { return checkedAt },
		ProbeDB:   func(context.Context) error { return nil },
	})

	got := svc.Snapshot(context.Background())

	if got.Status != "ok" {
		t.Fatalf("expected status ok, got %q", got.Status)
	}
	if got.DB != "up" {
		t.Fatalf("expected db up, got %q", got.DB)
	}
	if got.UptimeSeconds != 42 {
		t.Fatalf("expected uptime_seconds 42, got %d", got.UptimeSeconds)
	}
	if got.StartedAt != "2026-02-24T12:00:00Z" {
		t.Fatalf("expected started_at RFC3339 UTC, got %q", got.StartedAt)
	}
	if got.CheckedAt != "2026-02-24T12:00:42Z" {
		t.Fatalf("expected checked_at RFC3339 UTC, got %q", got.CheckedAt)
	}
}

func TestHealthServiceSnapshot_DegradedWhenDBProbeFails(t *testing.T) {
	svc := NewHealthService(HealthServiceDeps{
		StartedAt: time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC),
		Now:       func() time.Time { return time.Date(2026, 2, 24, 12, 1, 0, 0, time.UTC) },
		ProbeDB:   func(context.Context) error { return errors.New("db down") },
	})

	got := svc.Snapshot(context.Background())

	if got.Status != "degraded" {
		t.Fatalf("expected status degraded, got %q", got.Status)
	}
	if got.DB != "down" {
		t.Fatalf("expected db down, got %q", got.DB)
	}
}

func TestHealthServiceSnapshot_ClampsNegativeUptime(t *testing.T) {
	startedAt := time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC)
	checkedAt := time.Date(2026, 2, 24, 11, 59, 50, 0, time.UTC)

	svc := NewHealthService(HealthServiceDeps{
		StartedAt: startedAt,
		Now:       func() time.Time { return checkedAt },
		ProbeDB:   func(context.Context) error { return nil },
	})

	got := svc.Snapshot(context.Background())

	if got.UptimeSeconds != 0 {
		t.Fatalf("expected uptime_seconds to clamp at 0, got %d", got.UptimeSeconds)
	}
}
