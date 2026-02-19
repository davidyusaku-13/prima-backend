package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"backend/internal/db"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"golang.org/x/time/rate"
)

type User struct {
	ID       int64  `json:"id"`
	ClerkID  string `json:"clerk_id,omitempty"`
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
	Username string `json:"username,omitempty"`
}

type ClerkWebhookEvent struct {
	Type string `json:"type"`
	Data struct {
		ID                    string `json:"id"`
		Username              string `json:"username"`
		FirstName             string `json:"first_name"`
		LastName              string `json:"last_name"`
		PrimaryEmailAddressID string `json:"primary_email_address_id"`
		EmailAddresses        []struct {
			ID           string `json:"id"`
			EmailAddress string `json:"email_address"`
		} `json:"email_addresses"`
	} `json:"data"`
}

func toText(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	return pgtype.Text{String: s, Valid: s != ""}
}

func pickClerkEmail(evt ClerkWebhookEvent) string {
	if evt.Data.PrimaryEmailAddressID != "" {
		for _, e := range evt.Data.EmailAddresses {
			if e.ID == evt.Data.PrimaryEmailAddressID && strings.TrimSpace(e.EmailAddress) != "" {
				return strings.ToLower(strings.TrimSpace(e.EmailAddress))
			}
		}
	}
	for _, e := range evt.Data.EmailAddresses {
		if strings.TrimSpace(e.EmailAddress) != "" {
			return strings.ToLower(strings.TrimSpace(e.EmailAddress))
		}
	}
	return ""
}

// Minimal Svix verification for Clerk webhooks.
func verifySvix(body []byte, secret, svixID, svixTimestamp, svixSignature string) bool {
	if secret == "" || svixID == "" || svixTimestamp == "" || svixSignature == "" {
		return false
	}

	parts := strings.SplitN(secret, "_", 2)
	if len(parts) != 2 {
		return false
	}
	key, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}

	msg := svixID + "." + svixTimestamp + "." + string(body)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(msg))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	for _, token := range strings.Split(svixSignature, " ") {
		p := strings.SplitN(token, ",", 2) // e.g. "v1,abc..."
		if len(p) == 2 && p[0] == "v1" && hmac.Equal([]byte(p[1]), []byte(expected)) {
			return true
		}
	}
	return false
}

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type limiterStore struct {
	mu      sync.Mutex
	clients map[string]*clientLimiter
	r       rate.Limit
	burst   int
}

func newLimiterStore(r rate.Limit, burst int) *limiterStore {
	ls := &limiterStore{
		clients: make(map[string]*clientLimiter),
		r:       r,
		burst:   burst,
	}

	go func() {
		t := time.NewTicker(2 * time.Minute)
		defer t.Stop()
		for range t.C {
			ls.mu.Lock()
			for ip, c := range ls.clients {
				if time.Since(c.lastSeen) > 10*time.Minute {
					delete(ls.clients, ip)
				}
			}
			ls.mu.Unlock()
		}
	}()

	return ls
}

func (ls *limiterStore) get(ip string) *rate.Limiter {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if c, ok := ls.clients[ip]; ok {
		c.lastSeen = time.Now()
		return c.limiter
	}

	lim := rate.NewLimiter(ls.r, ls.burst)
	ls.clients[ip] = &clientLimiter{limiter: lim, lastSeen: time.Now()}
	return lim
}

func rateLimitMiddleware(ls *limiterStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !ls.get(ip).Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func main() {
	_ = godotenv.Load()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		panic("DATABASE_URL is required")
	}
	webhookSecret := os.Getenv("CLERK_WEBHOOK_SECRET")

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

	q := db.New(pool)

	r := gin.Default()
	r.Use(rateLimitMiddleware(newLimiterStore(10, 20))) // 10 req/sec per IP, burst 20

	r.GET("/health", func(c *gin.Context) {
		var v int
		if err := pool.QueryRow(c.Request.Context(), "SELECT 1").Scan(&v); err != nil {
			c.JSON(http.StatusOK, gin.H{"status": "degraded", "db": "down"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "db": "up"})
	})

	r.GET("/users", func(c *gin.Context) {
		users, err := q.ListUsers(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if users == nil {
			users = []db.ListUsersRow{}
		}
		c.JSON(http.StatusOK, users)
	})

	r.POST("/webhooks/clerk", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}

		if !verifySvix(
			body,
			webhookSecret,
			c.GetHeader("svix-id"),
			c.GetHeader("svix-timestamp"),
			c.GetHeader("svix-signature"),
		) {
			c.Status(http.StatusUnauthorized)
			return
		}

		var evt ClerkWebhookEvent
		if err := json.Unmarshal(body, &evt); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(evt.Data.FirstName + " " + evt.Data.LastName)
		if name == "" {
			name = strings.TrimSpace(evt.Data.Username)
		}
		if name == "" {
			name = "User"
		}
		email := pickClerkEmail(evt)

		switch evt.Type {
		case "user.created", "user.updated":
			if strings.TrimSpace(evt.Data.ID) == "" {
				c.JSON(http.StatusOK, gin.H{"ok": true, "ignored": "missing id", "type": evt.Type})
				return
			}
			if err := q.UpsertByClerkID(c.Request.Context(), db.UpsertByClerkIDParams{
				ClerkID:  strings.TrimSpace(evt.Data.ID),
				Username: toText(evt.Data.Username),
				Name:     name,
				Email:    toText(email),
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		case "user.deleted":
			if strings.TrimSpace(evt.Data.ID) != "" {
				if err := q.DeleteUserByClerkID(c.Request.Context(), strings.TrimSpace(evt.Data.ID)); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{"ok": true, "type": evt.Type})
	})

	if err := r.Run(":8080"); err != nil {
		panic(err)
	}
}
