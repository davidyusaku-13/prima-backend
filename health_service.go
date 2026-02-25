package main

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	healthStatusOK       = "ok"
	healthStatusDegraded = "degraded"
	healthStatusDown     = "down"
)

type HealthComponentSnapshot struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	LatencyMS     int64  `json:"latency_ms"`
	CheckedAt     string `json:"checked_at"`
	LastSuccessAt string `json:"last_success_at"`
	Error         string `json:"error"`
}

type AdminHealthSnapshot struct {
	Status        string                    `json:"status"`
	Summary       string                    `json:"summary"`
	DB            string                    `json:"db"`
	StartedAt     string                    `json:"started_at"`
	CheckedAt     string                    `json:"checked_at"`
	UptimeSeconds int64                     `json:"uptime_seconds"`
	Version       string                    `json:"version,omitempty"`
	Components    []HealthComponentSnapshot `json:"components"`
}

type HealthServiceDeps struct {
	StartedAt    time.Time
	Version      string
	Now          func() time.Time
	ProbeDB      func(context.Context) error
	ProbeTimeout time.Duration
}

type HealthService struct {
	startedAt    time.Time
	version      string
	now          func() time.Time
	probeDB      func(context.Context) error
	probeTimeout time.Duration

	mu              sync.RWMutex
	lastDBSuccessAt time.Time
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

	probeTimeout := deps.ProbeTimeout
	if probeTimeout <= 0 {
		probeTimeout = 2 * time.Second
	}

	return &HealthService{
		startedAt:    startedAt.UTC(),
		version:      deps.Version,
		now:          now,
		probeDB:      probeDB,
		probeTimeout: probeTimeout,
	}
}

func (s *HealthService) Snapshot(ctx context.Context) AdminHealthSnapshot {
	checkedAt := s.now().UTC()
	status := healthStatusOK
	db := "up"
	componentStatus := "up"
	componentError := ""

	probeCtx := ctx
	cancel := func() {}
	if s.probeTimeout > 0 {
		probeCtx, cancel = context.WithTimeout(ctx, s.probeTimeout)
	}
	defer cancel()

	probeStartedAt := time.Now()
	if err := s.probeDB(probeCtx); err != nil {
		status = healthStatusDegraded
		db = "down"
		componentStatus = "down"
		componentError = sanitizeHealthError(err)
	} else {
		s.setLastDBSuccessAt(checkedAt)
	}

	latencyMS := time.Since(probeStartedAt).Milliseconds()
	if latencyMS < 0 {
		latencyMS = 0
	}

	uptimeSeconds := int64(checkedAt.Sub(s.startedAt).Seconds())
	if uptimeSeconds < 0 {
		uptimeSeconds = 0
	}

	lastDBSuccessAt := s.getLastDBSuccessAt()
	lastDBSuccessAtValue := ""
	if !lastDBSuccessAt.IsZero() {
		lastDBSuccessAtValue = lastDBSuccessAt.Format(time.RFC3339)
	}

	checkedAtValue := checkedAt.Format(time.RFC3339)
	components := []HealthComponentSnapshot{
		{
			Name:          "database",
			Status:        componentStatus,
			LatencyMS:     latencyMS,
			CheckedAt:     checkedAtValue,
			LastSuccessAt: lastDBSuccessAtValue,
			Error:         componentError,
		},
	}

	return AdminHealthSnapshot{
		Status:        status,
		Summary:       buildHealthSummary(status, db, componentError),
		DB:            db,
		StartedAt:     s.startedAt.Format(time.RFC3339),
		CheckedAt:     checkedAtValue,
		UptimeSeconds: uptimeSeconds,
		Version:       s.version,
		Components:    components,
	}
}

func (s *HealthService) setLastDBSuccessAt(t time.Time) {
	s.mu.Lock()
	s.lastDBSuccessAt = t.UTC()
	s.mu.Unlock()
}

func (s *HealthService) getLastDBSuccessAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastDBSuccessAt
}

func sanitizeHealthError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return "database probe timed out"
	case errors.Is(err, context.Canceled):
		return "database probe canceled"
	default:
		return "database probe failed"
	}
}

func buildHealthSummary(status, db, dbError string) string {
	switch {
	case status == healthStatusOK:
		return "All monitored components are healthy."
	case db == "down" && dbError != "":
		return "Database check failed. " + dbError + "."
	case db == "down":
		return "Database check failed."
	case status == healthStatusDown:
		return "Service health is down."
	default:
		return "One or more components are degraded."
	}
}
