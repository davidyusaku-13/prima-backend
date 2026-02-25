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
		StartedAt:    startedAt,
		Version:      "test-version",
		ProbeTimeout: time.Second,
		Now:          func() time.Time { return checkedAt },
		ProbeDB:      func(context.Context) error { return nil },
	})

	got := svc.Snapshot(context.Background())

	if got.Status != healthStatusOK {
		t.Fatalf("expected status ok, got %q", got.Status)
	}
	if got.DB != "up" {
		t.Fatalf("expected db up, got %q", got.DB)
	}
	if got.Summary != "All monitored components are healthy." {
		t.Fatalf("expected healthy summary, got %q", got.Summary)
	}
	if got.Version != "test-version" {
		t.Fatalf("expected version test-version, got %q", got.Version)
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
	if len(got.Components) != 1 {
		t.Fatalf("expected one component snapshot, got %d", len(got.Components))
	}

	component := got.Components[0]
	if component.Name != "database" {
		t.Fatalf("expected component database, got %q", component.Name)
	}
	if component.Status != "up" {
		t.Fatalf("expected component status up, got %q", component.Status)
	}
	if component.CheckedAt != got.CheckedAt {
		t.Fatalf("expected component checked_at to match snapshot checked_at, got %q", component.CheckedAt)
	}
	if component.LastSuccessAt != got.CheckedAt {
		t.Fatalf("expected component last_success_at to match checked_at, got %q", component.LastSuccessAt)
	}
	if component.Error != "" {
		t.Fatalf("expected empty component error, got %q", component.Error)
	}
	if component.LatencyMS < 0 {
		t.Fatalf("expected non-negative latency, got %d", component.LatencyMS)
	}
}

func TestHealthServiceSnapshot_DegradedWhenDBProbeFails(t *testing.T) {
	svc := NewHealthService(HealthServiceDeps{
		StartedAt:    time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC),
		ProbeTimeout: time.Second,
		Now:          func() time.Time { return time.Date(2026, 2, 24, 12, 1, 0, 0, time.UTC) },
		ProbeDB:      func(context.Context) error { return errors.New("db down") },
	})

	got := svc.Snapshot(context.Background())

	if got.Status != healthStatusDegraded {
		t.Fatalf("expected status degraded, got %q", got.Status)
	}
	if got.DB != "down" {
		t.Fatalf("expected db down, got %q", got.DB)
	}
	if got.Summary != "Database check failed. database probe failed." {
		t.Fatalf("expected degraded summary, got %q", got.Summary)
	}
	if len(got.Components) != 1 {
		t.Fatalf("expected one component snapshot, got %d", len(got.Components))
	}
	component := got.Components[0]
	if component.Status != "down" {
		t.Fatalf("expected component down, got %q", component.Status)
	}
	if component.Error != "database probe failed" {
		t.Fatalf("expected sanitized error, got %q", component.Error)
	}
	if component.LastSuccessAt != "" {
		t.Fatalf("expected empty last_success_at before any success, got %q", component.LastSuccessAt)
	}
}

func TestHealthServiceSnapshot_ClampsNegativeUptime(t *testing.T) {
	startedAt := time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC)
	checkedAt := time.Date(2026, 2, 24, 11, 59, 50, 0, time.UTC)

	svc := NewHealthService(HealthServiceDeps{
		StartedAt:    startedAt,
		ProbeTimeout: time.Second,
		Now:          func() time.Time { return checkedAt },
		ProbeDB:      func(context.Context) error { return nil },
	})

	got := svc.Snapshot(context.Background())

	if got.UptimeSeconds != 0 {
		t.Fatalf("expected uptime_seconds to clamp at 0, got %d", got.UptimeSeconds)
	}
}

func TestHealthServiceSnapshot_PreservesLastSuccessOnFailure(t *testing.T) {
	startedAt := time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC)
	firstCheckedAt := time.Date(2026, 2, 24, 12, 0, 10, 0, time.UTC)
	secondCheckedAt := time.Date(2026, 2, 24, 12, 1, 0, 0, time.UTC)

	nowCalls := 0
	now := func() time.Time {
		nowCalls++
		if nowCalls == 1 {
			return firstCheckedAt
		}
		return secondCheckedAt
	}

	probeCalls := 0
	probeDB := func(context.Context) error {
		probeCalls++
		if probeCalls == 1 {
			return nil
		}
		return errors.New("db down")
	}

	svc := NewHealthService(HealthServiceDeps{
		StartedAt:    startedAt,
		ProbeTimeout: time.Second,
		Now:          now,
		ProbeDB:      probeDB,
	})

	firstSnapshot := svc.Snapshot(context.Background())
	if firstSnapshot.Status != healthStatusOK {
		t.Fatalf("expected first snapshot to be healthy, got %q", firstSnapshot.Status)
	}

	secondSnapshot := svc.Snapshot(context.Background())
	if secondSnapshot.Status != healthStatusDegraded {
		t.Fatalf("expected second snapshot to be degraded, got %q", secondSnapshot.Status)
	}
	if len(secondSnapshot.Components) != 1 {
		t.Fatalf("expected one component snapshot, got %d", len(secondSnapshot.Components))
	}
	if secondSnapshot.Components[0].LastSuccessAt != firstCheckedAt.Format(time.RFC3339) {
		t.Fatalf(
			"expected last_success_at %q, got %q",
			firstCheckedAt.Format(time.RFC3339),
			secondSnapshot.Components[0].LastSuccessAt,
		)
	}
}
