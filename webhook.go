package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"backend/internal/db"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
)

type ClerkUserWebhookEvent struct {
	Type string `json:"type"`
	Data struct {
		ID                    string `json:"id"`
		UserID                string `json:"user_id"`
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

type webhookQueries interface {
	UpsertUserWithRole(ctx context.Context, arg db.UpsertUserWithRoleParams) error
	SoftDeleteUserByClerkID(ctx context.Context, clerkID string) error
	UpdateLastLogin(ctx context.Context, clerkID string) error
}

type webhookHandleResult struct {
	Ignored string
}

const webhookTimestampTolerance = 5 * time.Minute

func toNullableText(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	return pgtype.Text{String: s, Valid: s != ""}
}

func pickClerkEmail(event ClerkUserWebhookEvent) string {
	if event.Data.PrimaryEmailAddressID != "" {
		for _, e := range event.Data.EmailAddresses {
			if e.ID == event.Data.PrimaryEmailAddressID && strings.TrimSpace(e.EmailAddress) != "" {
				return strings.ToLower(strings.TrimSpace(e.EmailAddress))
			}
		}
	}
	for _, e := range event.Data.EmailAddresses {
		if strings.TrimSpace(e.EmailAddress) != "" {
			return strings.ToLower(strings.TrimSpace(e.EmailAddress))
		}
	}
	return ""
}

// Minimal Svix verification for Clerk webhooks.
func verifySvixSignature(body []byte, secret, svixID, svixTimestamp, svixSignature string, now time.Time) bool {
	if secret == "" || svixID == "" || svixTimestamp == "" || svixSignature == "" {
		return false
	}
	timestampUnix, err := strconv.ParseInt(strings.TrimSpace(svixTimestamp), 10, 64)
	if err != nil {
		return false
	}
	timestamp := time.Unix(timestampUnix, 0).UTC()
	if timestamp.Before(now.Add(-webhookTimestampTolerance)) || timestamp.After(now.Add(webhookTimestampTolerance)) {
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
		parts := strings.SplitN(token, ",", 2) // e.g. "v1,abc..."
		if len(parts) == 2 && parts[0] == "v1" && hmac.Equal([]byte(parts[1]), []byte(expected)) {
			return true
		}
	}
	return false
}

func registerClerkWebhookRoutes(router *gin.Engine, queries *db.Queries, webhookSecret string) {
	router.POST("/webhooks/clerk", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}

		if !verifySvixSignature(
			body,
			webhookSecret,
			c.GetHeader("svix-id"),
			c.GetHeader("svix-timestamp"),
			c.GetHeader("svix-signature"),
			time.Now().UTC(),
		) {
			c.Status(http.StatusUnauthorized)
			return
		}

		var event ClerkUserWebhookEvent
		if err := json.Unmarshal(body, &event); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}

		result, err := handleClerkWebhookEvent(c.Request.Context(), queries, event)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		response := gin.H{"ok": true, "type": event.Type}
		if result.Ignored != "" {
			response["ignored"] = result.Ignored
		}
		c.JSON(http.StatusOK, response)
	})
}

func handleClerkWebhookEvent(ctx context.Context, queries webhookQueries, event ClerkUserWebhookEvent) (webhookHandleResult, error) {
	name := strings.TrimSpace(strings.TrimSpace(event.Data.FirstName) + " " + strings.TrimSpace(event.Data.LastName))
	if name == "" {
		name = strings.TrimSpace(event.Data.Username)
	}
	if name == "" {
		name = "User"
	}
	email := pickClerkEmail(event)

	switch event.Type {
	case "user.created", "user.updated":
		if strings.TrimSpace(event.Data.ID) == "" {
			return webhookHandleResult{Ignored: "missing id"}, nil
		}
		if err := queries.UpsertUserWithRole(ctx, db.UpsertUserWithRoleParams{
			ClerkID:   strings.TrimSpace(event.Data.ID),
			Username:  toNullableText(event.Data.Username),
			Name:      name,
			Email:     toNullableText(email),
			FirstName: toNullableText(event.Data.FirstName),
			LastName:  toNullableText(event.Data.LastName),
		}); err != nil {
			return webhookHandleResult{}, err
		}
	case "user.deleted":
		if strings.TrimSpace(event.Data.ID) == "" {
			return webhookHandleResult{}, nil
		}
		if err := queries.SoftDeleteUserByClerkID(ctx, strings.TrimSpace(event.Data.ID)); err != nil {
			return webhookHandleResult{}, err
		}
	case "session.created":
		userID := strings.TrimSpace(event.Data.UserID)
		if userID == "" {
			return webhookHandleResult{Ignored: "missing user_id"}, nil
		}
		if err := queries.UpdateLastLogin(ctx, userID); err != nil {
			return webhookHandleResult{}, err
		}
	}

	return webhookHandleResult{}, nil
}
