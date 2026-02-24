package main

import (
	"net/http"

	"backend/internal/db"

	"github.com/gin-gonic/gin"
)

func registerAdminRoutes(router *gin.Engine, queries *db.Queries, healthService *HealthService) {
	admin := router.Group("/admin")
	admin.Use(clerkAuthMiddleware(queries))

	admin.GET("/health", func(c *gin.Context) {
		snapshot := healthService.Snapshot(c.Request.Context())
		c.JSON(http.StatusOK, snapshot)
	})

	admin.GET("", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "section": "admin"})
	})

	admin.GET("/users", func(c *gin.Context) {
		users, err := queries.ListUsers(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve users"})
			return
		}
		if users == nil {
			users = []db.ListUsersRow{}
		}
		c.JSON(http.StatusOK, users)
	})
}
