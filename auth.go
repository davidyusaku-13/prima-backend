package main

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"backend/internal/db"

	"github.com/clerk/clerk-sdk-go/v2/jwt"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type authIdentity struct {
	ClerkID string
}

type authQueries interface {
	GetUserAuthContext(ctx context.Context, clerkID string) (db.GetUserAuthContextRow, error)
	UpdateLastLogin(ctx context.Context, clerkID string) error
}

type identityVerifier func(c *gin.Context) (authIdentity, error)

func parseBearerToken(authHeader string) string {
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
}

func verifyClerkIdentityFromRequest(c *gin.Context) (authIdentity, error) {
	token := parseBearerToken(c.GetHeader("Authorization"))
	if token == "" {
		return authIdentity{}, errors.New("missing authorization header")
	}

	claims, err := jwt.Verify(c.Request.Context(), &jwt.VerifyParams{Token: token})
	if err != nil {
		return authIdentity{}, errors.New("invalid or expired token")
	}

	clerkID := strings.TrimSpace(claims.Subject)
	if clerkID == "" {
		return authIdentity{}, errors.New("invalid token subject")
	}

	return authIdentity{ClerkID: clerkID}, nil
}

// clerkAuthMiddleware verifies the Clerk session JWT and enforces that the
// caller has role "superadmin" or hospital-scoped "admin".
func clerkAuthMiddleware(queries *db.Queries) gin.HandlerFunc {
	return clerkAuthMiddlewareWithIdentityVerifier(queries, verifyClerkIdentityFromRequest)
}

func clerkAuthMiddlewareWithIdentityVerifier(queries authQueries, verify identityVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, err := verify(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		authCtx, err := queries.GetUserAuthContext(c.Request.Context(), identity.ClerkID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "user not found or inactive"})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to load auth context"})
			return
		}

		switch authCtx.GlobalRole {
		case "superadmin":
			// unrestricted.
		case "admin":
			if authCtx.HospitalID <= 0 || authCtx.MembershipRole != "admin" {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin is not assigned to an active hospital"})
				return
			}
		default:
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}

		_ = queries.UpdateLastLogin(c.Request.Context(), authCtx.ClerkID)

		c.Set("clerk_id", authCtx.ClerkID)
		c.Set("role", authCtx.GlobalRole)
		c.Set("hospital_id", authCtx.HospitalID)
		c.Set("membership_role", authCtx.MembershipRole)
		c.Next()
	}
}
