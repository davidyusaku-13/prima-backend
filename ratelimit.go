package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

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
	store := &limiterStore{
		clients: make(map[string]*clientLimiter),
		r:       r,
		burst:   burst,
	}

	go func() {
		t := time.NewTicker(2 * time.Minute)
		defer t.Stop()
		for range t.C {
			store.mu.Lock()
			for ip, client := range store.clients {
				if time.Since(client.lastSeen) > 10*time.Minute {
					delete(store.clients, ip)
				}
			}
			store.mu.Unlock()
		}
	}()

	return store
}

func (store *limiterStore) get(ip string) *rate.Limiter {
	store.mu.Lock()
	defer store.mu.Unlock()

	if client, ok := store.clients[ip]; ok {
		client.lastSeen = time.Now()
		return client.limiter
	}

	limiter := rate.NewLimiter(store.r, store.burst)
	store.clients[ip] = &clientLimiter{limiter: limiter, lastSeen: time.Now()}
	return limiter
}

func rateLimitMiddleware(store *limiterStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		ip := c.ClientIP()
		if !store.get(ip).Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			c.Abort()
			return
		}
		c.Next()
	}
}
