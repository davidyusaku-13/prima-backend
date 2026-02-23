package main

import (
	"net/http"
	"strings"

	"backend/internal/db"

	"github.com/clerk/clerk-sdk-go/v2/jwt"
	"github.com/gin-gonic/gin"
)

// clerkAuthMiddleware verifies the Clerk session JWT and enforces that the
// caller has role "superadmin" or "admin" in the database.
func clerkAuthMiddleware(queries *db.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")

		claims, err := jwt.Verify(c.Request.Context(), &jwt.VerifyParams{Token: token})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		clerkID := claims.Subject
		if clerkID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token subject"})
			return
		}

		role, err := queries.GetUserRole(c.Request.Context(), clerkID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "user not found or inactive"})
			return
		}

		if role != "superadmin" && role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}

		c.Set("clerk_id", clerkID)
		c.Set("role", role)
		c.Next()
	}
}
