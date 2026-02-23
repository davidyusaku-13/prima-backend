package main

import (
	"net/http"

	"backend/internal/db"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerAdminRoutes(router *gin.Engine, queries *db.Queries, pool *pgxpool.Pool) {
	admin := router.Group("/admin")
	admin.Use(clerkAuthMiddleware(queries))

	admin.GET("/health", func(c *gin.Context) {
		var dbProbe int
		if err := pool.QueryRow(c.Request.Context(), "SELECT 1").Scan(&dbProbe); err != nil {
			c.JSON(http.StatusOK, gin.H{"status": "degraded", "db": "down"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "db": "up"})
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
