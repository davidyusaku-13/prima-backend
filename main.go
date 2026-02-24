package main

import (
	"context"
	"os"
	"time"

	"backend/internal/db"

	clerkSDK "github.com/clerk/clerk-sdk-go/v2"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	startedAt := time.Now().UTC()

	_ = godotenv.Load()

	clerkSecretKey := os.Getenv("CLERK_SECRET_KEY")
	if clerkSecretKey == "" {
		panic("CLERK_SECRET_KEY is not set")
	}
	clerkSDK.SetKey(clerkSecretKey)

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		panic("DATABASE_URL is required")
	}
	webhookSecret := os.Getenv("CLERK_WEBHOOK_SECRET")
	if webhookSecret == "" {
		panic("CLERK_WEBHOOK_SECRET is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		panic(err)
	}

	queries := db.New(pool)
	healthService := NewHealthService(HealthServiceDeps{
		StartedAt: startedAt,
		ProbeDB: func(ctx context.Context) error {
			var dbProbe int
			return pool.QueryRow(ctx, "SELECT 1").Scan(&dbProbe)
		},
	})

	router := gin.Default()
	router.SetTrustedProxies([]string{"127.0.0.1:8787"})
	router.Use(rateLimitMiddleware(newLimiterStore(10, 20))) // 10 req/sec per IP, burst 20

	registerAdminRoutes(router, queries, healthService)
	registerClerkWebhookRoutes(router, queries, webhookSecret)

	if err := router.Run(":8080"); err != nil {
		panic(err)
	}
}
