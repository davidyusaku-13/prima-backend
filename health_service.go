package main

import (
	"context"
	"time"
)

type AdminHealthSnapshot struct {
	Status        string `json:"status"`
	DB            string `json:"db"`
	StartedAt     string `json:"started_at"`
	CheckedAt     string `json:"checked_at"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

type HealthServiceDeps struct {
	StartedAt time.Time
	Now       func() time.Time
	ProbeDB   func(context.Context) error
}

type HealthService struct {
	startedAt time.Time
	now       func() time.Time
	probeDB   func(context.Context) error
}

func NewHealthService(deps HealthServiceDeps) *HealthService {
	startedAt := deps.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}

	now := deps.Now
	if now == nil {
		now = time.Now
	}

	probeDB := deps.ProbeDB
	if probeDB == nil {
		probeDB = func(context.Context) error { return nil }
	}

	return &HealthService{
		startedAt: startedAt.UTC(),
		now:       now,
		probeDB:   probeDB,
	}
}

func (s *HealthService) Snapshot(ctx context.Context) AdminHealthSnapshot {
	checkedAt := s.now().UTC()
	status := "ok"
	db := "up"

	if err := s.probeDB(ctx); err != nil {
		status = "degraded"
		db = "down"
	}

	uptimeSeconds := int64(checkedAt.Sub(s.startedAt).Seconds())
	if uptimeSeconds < 0 {
		uptimeSeconds = 0
	}

	return AdminHealthSnapshot{
		Status:        status,
		DB:            db,
		StartedAt:     s.startedAt.Format(time.RFC3339),
		CheckedAt:     checkedAt.Format(time.RFC3339),
		UptimeSeconds: uptimeSeconds,
	}
}
