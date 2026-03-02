package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/db"

	"github.com/gin-gonic/gin"
)

type authMiddlewareQueriesStub struct {
	authCtx    db.GetUserAuthContextRow
	getAuthErr error
}

func (s *authMiddlewareQueriesStub) GetUserAuthContext(_ context.Context, _ string) (db.GetUserAuthContextRow, error) {
	if s.getAuthErr != nil {
		return db.GetUserAuthContextRow{}, s.getAuthErr
	}
	return s.authCtx, nil
}

func TestClerkAuthMiddleware_AllowsAuthorizedUser(t *testing.T) {
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
}

func TestClerkAuthMiddleware_RejectsForbiddenUser(t *testing.T) {
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
}
