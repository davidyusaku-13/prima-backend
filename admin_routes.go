package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"backend/internal/db"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var hospitalSlugPattern = regexp.MustCompile(`[^a-z0-9]+`)

type createHospitalRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type assignHospitalAdminRequest struct {
	UserClerkID string `json:"user_clerk_id"`
}

type createHospitalInviteRequest struct {
	ExpiresInHours int `json:"expires_in_hours"`
}

type inviteClaimRequest struct {
	Token string `json:"token"`
}

type adminAssignmentCandidate struct {
	ClerkID                string `json:"clerk_id"`
	Name                   string `json:"name"`
	Email                  string `json:"email"`
	Username               string `json:"username"`
	Role                   string `json:"role"`
	IsActive               bool   `json:"is_active"`
	MembershipHospitalID   int64  `json:"membership_hospital_id"`
	MembershipHospitalSlug string `json:"membership_hospital_slug"`
	MembershipRole         string `json:"membership_role"`
	Assignable             bool   `json:"assignable"`
	Status                 string `json:"status"`
}

func normalizeHospitalSlug(name, slug string) string {
	base := strings.TrimSpace(slug)
	if base == "" {
		base = strings.TrimSpace(name)
	}
	base = strings.ToLower(base)
	base = hospitalSlugPattern.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if len(base) > 64 {
		base = strings.Trim(base[:64], "-")
	}
	return base
}

func generateInviteToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashInviteToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func authRole(c *gin.Context) string {
	return c.GetString("role")
}

func authClerkID(c *gin.Context) string {
	return c.GetString("clerk_id")
}

func authHospitalID(c *gin.Context) int64 {
	return c.GetInt64("hospital_id")
}

func hospitalAccessDenied(c *gin.Context, hospitalID int64) bool {
	if authRole(c) == "superadmin" {
		return false
	}
	return authHospitalID(c) != hospitalID
}

func loadHospitalForAdminAccess(c *gin.Context, queries *db.Queries, slug string) (db.Hospital, bool) {
	hospital, err := queries.GetHospitalBySlug(c.Request.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "hospital not found"})
			return db.Hospital{}, false
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load hospital"})
		return db.Hospital{}, false
	}
	if !hospital.IsActive {
		c.JSON(http.StatusNotFound, gin.H{"error": "hospital is inactive"})
		return db.Hospital{}, false
	}
	if hospitalAccessDenied(c, hospital.ID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for hospital"})
		return db.Hospital{}, false
	}
	return hospital, true
}

func registerAdminRoutes(router *gin.Engine, queries *db.Queries, pool *pgxpool.Pool, healthService *HealthService) {
	admin := router.Group("/admin")
	admin.Use(clerkAuthMiddleware(queries))

	admin.GET("/health", func(c *gin.Context) {
		snapshot := healthService.Snapshot(c.Request.Context())
		c.JSON(healthHTTPStatusCode(snapshot.Status), snapshot)
	})

	admin.GET("", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":      "ok",
			"section":     "admin",
			"role":        authRole(c),
			"hospital_id": authHospitalID(c),
		})
	})

	admin.GET("/users", func(c *gin.Context) {
		if authRole(c) != "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "only superadmin can list all users"})
			return
		}

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

	admin.GET("/hospitals", func(c *gin.Context) {
		if authRole(c) == "superadmin" {
			hospitals, err := queries.ListHospitals(c.Request.Context())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve hospitals"})
				return
			}
			if hospitals == nil {
				hospitals = []db.Hospital{}
			}
			c.JSON(http.StatusOK, hospitals)
			return
		}

		hospital, err := queries.GetHospitalByID(c.Request.Context(), authHospitalID(c))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusForbidden, gin.H{"error": "admin hospital assignment is invalid"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve hospital"})
			return
		}
		if !hospital.IsActive {
			c.JSON(http.StatusForbidden, gin.H{"error": "assigned hospital is inactive"})
			return
		}
		c.JSON(http.StatusOK, []db.Hospital{hospital})
	})

	admin.POST("/hospitals", func(c *gin.Context) {
		if authRole(c) != "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "only superadmin can create hospitals"})
			return
		}

		var payload createHospitalRequest
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}

		payload.Name = strings.TrimSpace(payload.Name)
		if payload.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
			return
		}

		slug := normalizeHospitalSlug(payload.Name, payload.Slug)
		if slug == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "slug is invalid"})
			return
		}

		hospital, err := queries.CreateHospital(c.Request.Context(), db.CreateHospitalParams{
			Name:             payload.Name,
			Slug:             slug,
			CreatedByClerkID: authClerkID(c),
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				c.JSON(http.StatusConflict, gin.H{"error": "hospital slug already exists"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create hospital"})
			return
		}

		c.JSON(http.StatusCreated, hospital)
	})

	admin.POST("/hospitals/:slug/admins", func(c *gin.Context) {
		if authRole(c) != "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "only superadmin can assign hospital admins"})
			return
		}

		hospital, ok := loadHospitalForAdminAccess(c, queries, c.Param("slug"))
		if !ok {
			return
		}

		var payload assignHospitalAdminRequest
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}

		payload.UserClerkID = strings.TrimSpace(payload.UserClerkID)
		if payload.UserClerkID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_clerk_id is required"})
			return
		}

		user, err := queries.GetUserByClerkID(c.Request.Context(), payload.UserClerkID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "target user not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load target user"})
			return
		}
		if user.DeletedAt.Valid {
			c.JSON(http.StatusNotFound, gin.H{"error": "target user is deleted"})
			return
		}
		if user.Role == "superadmin" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "superadmin cannot be assigned as hospital admin"})
			return
		}

		activeMembership, err := queries.GetActiveHospitalMembershipByUser(c.Request.Context(), payload.UserClerkID)
		if err == nil {
			if activeMembership.HospitalID != hospital.ID {
				c.JSON(http.StatusConflict, gin.H{"error": "user already belongs to another hospital"})
				return
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate existing membership"})
			return
		}

		if err := queries.AssignHospitalAdmin(c.Request.Context(), db.AssignHospitalAdminParams{
			HospitalID:       hospital.ID,
			UserClerkID:      payload.UserClerkID,
			InvitedByClerkID: toNullableText(authClerkID(c)),
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to assign hospital admin"})
			return
		}

		if user.Role != "superadmin" {
			if err := queries.SetUserRoleByClerkID(c.Request.Context(), db.SetUserRoleByClerkIDParams{
				ClerkID: payload.UserClerkID,
				Role:    "admin",
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user role"})
				return
			}
		}

		if err := queries.SetUserActiveByClerkID(c.Request.Context(), db.SetUserActiveByClerkIDParams{
			ClerkID:  payload.UserClerkID,
			IsActive: true,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to activate user"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	admin.DELETE("/hospitals/:slug/admins/:userClerkID", func(c *gin.Context) {
		if authRole(c) != "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "only superadmin can remove hospital admins"})
			return
		}

		hospital, ok := loadHospitalForAdminAccess(c, queries, c.Param("slug"))
		if !ok {
			return
		}

		targetClerkID := strings.TrimSpace(c.Param("userClerkID"))
		if targetClerkID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_clerk_id is required"})
			return
		}

		user, err := queries.GetUserByClerkID(c.Request.Context(), targetClerkID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "target user not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load target user"})
			return
		}
		if user.DeletedAt.Valid {
			c.JSON(http.StatusNotFound, gin.H{"error": "target user is deleted"})
			return
		}
		if user.Role == "superadmin" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "superadmin cannot be removed as hospital admin"})
			return
		}

		affectedRows, err := queries.DeactivateHospitalAdminMembership(c.Request.Context(), db.DeactivateHospitalAdminMembershipParams{
			HospitalID:  hospital.ID,
			UserClerkID: targetClerkID,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove hospital admin"})
			return
		}
		if affectedRows == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "target user is not an admin of this hospital"})
			return
		}

		hasActiveMembership, err := queries.HasActiveHospitalMembership(c.Request.Context(), targetClerkID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate active membership"})
			return
		}
		hasAdminMembership, err := queries.HasActiveAdminMembership(c.Request.Context(), targetClerkID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate admin membership"})
			return
		}

		if user.Role == "admin" && !hasAdminMembership {
			if err := queries.SetUserRoleByClerkID(c.Request.Context(), db.SetUserRoleByClerkIDParams{
				ClerkID: targetClerkID,
				Role:    "user",
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to downgrade user role"})
				return
			}
		}

		if !hasActiveMembership {
			if err := queries.SetUserActiveByClerkID(c.Request.Context(), db.SetUserActiveByClerkIDParams{
				ClerkID:  targetClerkID,
				IsActive: false,
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to deactivate user"})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	admin.GET("/hospitals/:slug/admins", func(c *gin.Context) {
		if authRole(c) != "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "only superadmin can list hospital admins"})
			return
		}

		slug := strings.TrimSpace(c.Param("slug"))
		if slug == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "hospital slug is required"})
			return
		}

		if _, ok := loadHospitalForAdminAccess(c, queries, slug); !ok {
			return
		}

		admins, err := queries.ListHospitalAdminsBySlug(c.Request.Context(), slug)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve hospital admins"})
			return
		}
		if admins == nil {
			admins = []db.ListHospitalAdminsBySlugRow{}
		}
		c.JSON(http.StatusOK, admins)
	})

	admin.GET("/hospitals/:slug/admin-candidates", func(c *gin.Context) {
		if authRole(c) != "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "only superadmin can list admin candidates"})
			return
		}

		slug := strings.TrimSpace(c.Param("slug"))
		if slug == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "hospital slug is required"})
			return
		}

		hospital, ok := loadHospitalForAdminAccess(c, queries, slug)
		if !ok {
			return
		}

		candidates, err := queries.ListAdminAssignmentCandidates(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve admin candidates"})
			return
		}

		response := make([]adminAssignmentCandidate, 0, len(candidates))
		for _, candidate := range candidates {
			entry := adminAssignmentCandidate{
				ClerkID:                candidate.ClerkID,
				Name:                   candidate.Name,
				Email:                  candidate.Email,
				Username:               candidate.Username,
				Role:                   candidate.Role,
				IsActive:               candidate.IsActive,
				MembershipHospitalID:   candidate.MembershipHospitalID,
				MembershipHospitalSlug: candidate.MembershipHospitalSlug,
				MembershipRole:         candidate.MembershipRole,
				Assignable:             false,
				Status:                 "assigned_elsewhere",
			}

			switch {
			case candidate.MembershipHospitalID == 0:
				entry.Assignable = true
				entry.Status = "unassigned"
			case candidate.MembershipHospitalID == hospital.ID && candidate.MembershipRole == "admin":
				entry.Assignable = false
				entry.Status = "already_admin"
			case candidate.MembershipHospitalID == hospital.ID:
				entry.Assignable = true
				entry.Status = "member"
			default:
				entry.Assignable = false
				entry.Status = "assigned_elsewhere"
			}

			response = append(response, entry)
		}

		c.JSON(http.StatusOK, response)
	})

	admin.GET("/hospitals/:slug/users", func(c *gin.Context) {
		slug := strings.TrimSpace(c.Param("slug"))
		if slug == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "hospital slug is required"})
			return
		}

		if _, ok := loadHospitalForAdminAccess(c, queries, slug); !ok {
			return
		}

		users, err := queries.ListHospitalUsersBySlug(c.Request.Context(), slug)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve hospital users"})
			return
		}
		if users == nil {
			users = []db.ListHospitalUsersBySlugRow{}
		}
		c.JSON(http.StatusOK, users)
	})

	admin.POST("/hospitals/:slug/invites", func(c *gin.Context) {
		slug := strings.TrimSpace(c.Param("slug"))
		if slug == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "hospital slug is required"})
			return
		}

		hospital, ok := loadHospitalForAdminAccess(c, queries, slug)
		if !ok {
			return
		}

		var payload createHospitalInviteRequest
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}
		if payload.ExpiresInHours <= 0 {
			payload.ExpiresInHours = 72
		}
		if payload.ExpiresInHours > 24*30 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "expires_in_hours cannot exceed 720"})
			return
		}

		token, err := generateInviteToken()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate invite token"})
			return
		}
		expiresAt := time.Now().UTC().Add(time.Duration(payload.ExpiresInHours) * time.Hour)

		invite, err := queries.CreateHospitalInvite(c.Request.Context(), db.CreateHospitalInviteParams{
			HospitalID:       hospital.ID,
			TokenHash:        hashInviteToken(token),
			CreatedByClerkID: authClerkID(c),
			ExpiresAt:        pgtype.Timestamptz{Time: expiresAt, Valid: true},
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create invite"})
			return
		}

		invitePath := "/register?invite=" + url.QueryEscape(token)
		c.JSON(http.StatusCreated, gin.H{
			"id":           invite.ID,
			"hospital_id":  invite.HospitalID,
			"invite_role":  invite.InviteRole,
			"expires_at":   invite.ExpiresAt.Time.UTC().Format(time.RFC3339),
			"invite_token": token,
			"invite_path":  invitePath,
		})
	})

	admin.GET("/hospitals/:slug/invites", func(c *gin.Context) {
		slug := strings.TrimSpace(c.Param("slug"))
		if slug == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "hospital slug is required"})
			return
		}

		if _, ok := loadHospitalForAdminAccess(c, queries, slug); !ok {
			return
		}

		invites, err := queries.ListHospitalInvitesBySlug(c.Request.Context(), slug)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve invites"})
			return
		}
		if invites == nil {
			invites = []db.ListHospitalInvitesBySlugRow{}
		}
		c.JSON(http.StatusOK, invites)
	})

	admin.POST("/hospitals/:slug/invites/:inviteID/replace-link", func(c *gin.Context) {
		slug := strings.TrimSpace(c.Param("slug"))
		if slug == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "hospital slug is required"})
			return
		}

		hospital, ok := loadHospitalForAdminAccess(c, queries, slug)
		if !ok {
			return
		}

		inviteID, err := strconv.ParseInt(strings.TrimSpace(c.Param("inviteID")), 10, 64)
		if err != nil || inviteID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invite id is invalid"})
			return
		}

		tx, err := pool.Begin(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start transaction"})
			return
		}
		defer tx.Rollback(c.Request.Context())
		qtx := queries.WithTx(tx)

		invite, err := qtx.GetHospitalInviteByIDForUpdate(c.Request.Context(), db.GetHospitalInviteByIDForUpdateParams{
			ID:         inviteID,
			HospitalID: hospital.ID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "invite not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load invite"})
			return
		}

		now := time.Now().UTC()
		if invite.ConsumedAt.Valid || invite.RevokedAt.Valid || !invite.ExpiresAt.Valid || !invite.ExpiresAt.Time.After(now) {
			c.JSON(http.StatusConflict, gin.H{"error": "invite is not active or already unavailable"})
			return
		}

		token, err := generateInviteToken()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate invite token"})
			return
		}

		remainingTTL := invite.ExpiresAt.Time.Sub(now)
		if remainingTTL <= 0 {
			remainingTTL = 72 * time.Hour
		}
		if remainingTTL < time.Hour {
			remainingTTL = time.Hour
		}
		if remainingTTL > 720*time.Hour {
			remainingTTL = 720 * time.Hour
		}
		newExpiresAt := now.Add(remainingTTL)

		revokedRows, err := qtx.RevokeHospitalInvite(c.Request.Context(), db.RevokeHospitalInviteParams{
			ID:               invite.ID,
			HospitalID:       hospital.ID,
			RevokedByClerkID: toNullableText(authClerkID(c)),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke invite"})
			return
		}
		if revokedRows == 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "invite is not active or already unavailable"})
			return
		}

		newInvite, err := qtx.CreateHospitalInvite(c.Request.Context(), db.CreateHospitalInviteParams{
			HospitalID:       hospital.ID,
			TokenHash:        hashInviteToken(token),
			CreatedByClerkID: authClerkID(c),
			ExpiresAt:        pgtype.Timestamptz{Time: newExpiresAt, Valid: true},
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create replacement invite"})
			return
		}

		if err := tx.Commit(c.Request.Context()); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to replace invite"})
			return
		}

		invitePath := "/register?invite=" + url.QueryEscape(token)
		c.JSON(http.StatusOK, gin.H{
			"old_invite_id": invite.ID,
			"new_invite_id": newInvite.ID,
			"invite_path":   invitePath,
			"expires_at":    newInvite.ExpiresAt.Time.UTC().Format(time.RFC3339),
		})
	})

	admin.DELETE("/hospitals/:slug/invites/:inviteID", func(c *gin.Context) {
		slug := strings.TrimSpace(c.Param("slug"))
		if slug == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "hospital slug is required"})
			return
		}

		hospital, ok := loadHospitalForAdminAccess(c, queries, slug)
		if !ok {
			return
		}

		inviteID, err := strconv.ParseInt(strings.TrimSpace(c.Param("inviteID")), 10, 64)
		if err != nil || inviteID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invite id is invalid"})
			return
		}

		affectedRows, err := queries.RevokeHospitalInvite(c.Request.Context(), db.RevokeHospitalInviteParams{
			ID:               inviteID,
			HospitalID:       hospital.ID,
			RevokedByClerkID: toNullableText(authClerkID(c)),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke invite"})
			return
		}
		if affectedRows == 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "invite is not active or already unavailable"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	registerInviteRoutes(router, queries, pool)
}

func registerInviteRoutes(router *gin.Engine, queries *db.Queries, pool *pgxpool.Pool) {
	router.POST("/invites/claim", func(c *gin.Context) {
		identity, err := verifyClerkIdentityFromRequest(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		var payload inviteClaimRequest
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}
		payload.Token = strings.TrimSpace(payload.Token)
		if payload.Token == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
			return
		}

		tx, err := pool.Begin(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start transaction"})
			return
		}
		defer tx.Rollback(c.Request.Context())
		qtx := queries.WithTx(tx)

		user, err := qtx.GetUserByClerkID(c.Request.Context(), identity.ClerkID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusConflict, gin.H{"error": "user profile is not provisioned yet"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load user"})
			return
		}
		if user.DeletedAt.Valid {
			c.JSON(http.StatusForbidden, gin.H{"error": "user account is deleted"})
			return
		}
		if user.Role == "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "superadmin cannot claim hospital invite"})
			return
		}

		invite, err := qtx.GetHospitalInviteByTokenHash(c.Request.Context(), hashInviteToken(payload.Token))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "invite token is invalid"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load invite"})
			return
		}
		if invite.HospitalDeletedAt.Valid || !invite.HospitalIsActive {
			c.JSON(http.StatusForbidden, gin.H{"error": "hospital is unavailable"})
			return
		}
		if invite.RevokedAt.Valid {
			c.JSON(http.StatusConflict, gin.H{"error": "invite revoked"})
			return
		}
		if invite.ConsumedAt.Valid {
			c.JSON(http.StatusConflict, gin.H{"error": "invite already used"})
			return
		}
		if !invite.ExpiresAt.Valid || time.Now().UTC().After(invite.ExpiresAt.Time) {
			c.JSON(http.StatusConflict, gin.H{"error": "invite expired"})
			return
		}

		hasMembership, err := qtx.HasActiveHospitalMembership(c.Request.Context(), identity.ClerkID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate membership"})
			return
		}
		if hasMembership {
			c.JSON(http.StatusConflict, gin.H{"error": "user already belongs to a hospital"})
			return
		}

		consumedRows, err := qtx.ConsumeHospitalInvite(c.Request.Context(), db.ConsumeHospitalInviteParams{
			ID:                invite.ID,
			ConsumedByClerkID: toNullableText(identity.ClerkID),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to claim invite"})
			return
		}
		if consumedRows == 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "invite is no longer claimable"})
			return
		}

		if err := qtx.UpsertHospitalUserMembership(c.Request.Context(), db.UpsertHospitalUserMembershipParams{
			HospitalID:       invite.HospitalID,
			UserClerkID:      identity.ClerkID,
			MembershipRole:   "user",
			InvitedByClerkID: toNullableText(invite.CreatedByClerkID),
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to attach hospital membership"})
			return
		}

		if user.Role != "admin" {
			if err := qtx.SetUserRoleByClerkID(c.Request.Context(), db.SetUserRoleByClerkIDParams{
				ClerkID: identity.ClerkID,
				Role:    "user",
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user role"})
				return
			}
		}
		if err := qtx.SetUserActiveByClerkID(c.Request.Context(), db.SetUserActiveByClerkIDParams{
			ClerkID:  identity.ClerkID,
			IsActive: true,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to activate user"})
			return
		}

		if err := tx.Commit(c.Request.Context()); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to commit invite claim"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"ok":          true,
			"hospital_id": invite.HospitalID,
			"role":        "user",
		})
	})
}

func healthHTTPStatusCode(status string) int {
	switch status {
	case healthStatusOK:
		return http.StatusOK
	case healthStatusDegraded, healthStatusDown:
		return http.StatusServiceUnavailable
	default:
		return http.StatusServiceUnavailable
	}
}
