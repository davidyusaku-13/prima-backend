package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/db"

	"github.com/gin-gonic/gin"
)

type authMiddlewareQueriesStub struct {
	authCtx      db.GetUserAuthContextRow
	getAuthErr   error
	updateErr    error
	updateCalls  int
	updatedClerk string
}

func (s *authMiddlewareQueriesStub) GetUserAuthContext(_ context.Context, _ string) (db.GetUserAuthContextRow, error) {
	if s.getAuthErr != nil {
		return db.GetUserAuthContextRow{}, s.getAuthErr
	}
	return s.authCtx, nil
}

func (s *authMiddlewareQueriesStub) UpdateLastLogin(_ context.Context, clerkID string) error {
	s.updateCalls++
	s.updatedClerk = clerkID
	return s.updateErr
}

func TestClerkAuthMiddleware_UpdatesLastLoginForAuthorizedUser(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	queries := &authMiddlewareQueriesStub{
		authCtx: db.GetUserAuthContextRow{
			ClerkID:        "user_123",
			GlobalRole:     "superadmin",
			HospitalID:     0,
			MembershipRole: "",
		},
	}

	r := gin.New()
	r.Use(clerkAuthMiddlewareWithIdentityVerifier(queries, func(c *gin.Context) (authIdentity, error) {
		return authIdentity{ClerkID: "user_123"}, nil
	}))
	r.GET("/admin", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if queries.updateCalls != 1 {
		t.Fatalf("expected UpdateLastLogin to be called once, got %d", queries.updateCalls)
	}
	if queries.updatedClerk != "user_123" {
		t.Fatalf("expected UpdateLastLogin to be called with clerk_id user_123, got %q", queries.updatedClerk)
	}
}

func TestClerkAuthMiddleware_DoesNotUpdateLastLoginWhenForbidden(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	queries := &authMiddlewareQueriesStub{
		authCtx: db.GetUserAuthContextRow{
			ClerkID:        "user_456",
			GlobalRole:     "user",
			HospitalID:     0,
			MembershipRole: "",
		},
	}

	r := gin.New()
	r.Use(clerkAuthMiddlewareWithIdentityVerifier(queries, func(c *gin.Context) (authIdentity, error) {
		return authIdentity{ClerkID: "user_456"}, nil
	}))
	r.GET("/admin", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}
	if queries.updateCalls != 0 {
		t.Fatalf("expected UpdateLastLogin not to be called, got %d", queries.updateCalls)
	}
}

func TestClerkAuthMiddleware_AllowsRequestWhenLastLoginUpdateFails(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	queries := &authMiddlewareQueriesStub{
		authCtx: db.GetUserAuthContextRow{
			ClerkID:        "user_789",
			GlobalRole:     "superadmin",
			HospitalID:     0,
			MembershipRole: "",
		},
		updateErr: errors.New("update failed"),
	}

	r := gin.New()
	r.Use(clerkAuthMiddlewareWithIdentityVerifier(queries, func(c *gin.Context) (authIdentity, error) {
		return authIdentity{ClerkID: "user_789"}, nil
	}))
	r.GET("/admin", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if queries.updateCalls != 1 {
		t.Fatalf("expected UpdateLastLogin to be called once, got %d", queries.updateCalls)
	}
}
