package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"backend/internal/db"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const adminUsersDefaultPageSize = 50
const adminUsersMaxPageSize = 200

type adminUsersOverviewResponse struct {
	TotalUsers                    int64                              `json:"total_users"`
	ActiveUsers                   int64                              `json:"active_users"`
	InactiveUsers                 int64                              `json:"inactive_users"`
	UnassignedUsers               int64                              `json:"unassigned_users"`
	AdminRoleMismatchUsers        int64                              `json:"admin_role_mismatch_users"`
	InactiveWithMembershipUsers   int64                              `json:"inactive_with_membership_users"`
	AssignedInactiveHospitalUsers int64                              `json:"assigned_inactive_hospital_users"`
	NeverLoggedInUsers            int64                              `json:"never_logged_in_users"`
	Hospitals                     []db.ListHospitalsForAdminUsersRow `json:"hospitals"`
}

type adminUsersSearchItem struct {
	ClerkID                  string   `json:"clerk_id"`
	Name                     string   `json:"name"`
	Email                    string   `json:"email"`
	Username                 string   `json:"username"`
	FirstName                string   `json:"first_name"`
	LastName                 string   `json:"last_name"`
	Role                     string   `json:"role"`
	IsActive                 bool     `json:"is_active"`
	CreatedAt                string   `json:"created_at"`
	UpdatedAt                string   `json:"updated_at"`
	LastLoginAt              string   `json:"last_login_at"`
	MembershipHospitalID     int64    `json:"membership_hospital_id"`
	MembershipHospitalSlug   string   `json:"membership_hospital_slug"`
	MembershipHospitalName   string   `json:"membership_hospital_name"`
	MembershipHospitalActive bool     `json:"membership_hospital_active"`
	MembershipRole           string   `json:"membership_role"`
	Anomalies                []string `json:"anomalies"`
}

type adminUsersSearchResponse struct {
	Items  []adminUsersSearchItem `json:"items"`
	Total  int64                  `json:"total"`
	Limit  int32                  `json:"limit"`
	Offset int32                  `json:"offset"`
}

type adminUserDetailResponse struct {
	User          adminUsersSearchItem     `json:"user"`
	RecentActions []adminUserActionSummary `json:"recent_actions"`
}

type adminUserActionSummary struct {
	ID           int64          `json:"id"`
	ActorClerkID string         `json:"actor_clerk_id"`
	ActionType   string         `json:"action_type"`
	Reason       string         `json:"reason"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    string         `json:"created_at"`
}

type adminUserReasonRequest struct {
	Reason string `json:"reason"`
}

type adminUserReassignHospitalRequest struct {
	HospitalSlug   string `json:"hospital_slug"`
	MembershipRole string `json:"membership_role"`
	Reason         string `json:"reason"`
}

type adminUserSetMembershipRoleRequest struct {
	MembershipRole string `json:"membership_role"`
	Reason         string `json:"reason"`
}

type adminUserSetActiveRequest struct {
	IsActive bool   `json:"is_active"`
	Reason   string `json:"reason"`
}

type adminUserBulkSetActiveRequest struct {
	ClerkIDs []string `json:"clerk_ids"`
	IsActive bool     `json:"is_active"`
	Reason   string   `json:"reason"`
}

type adminUserBulkSetActiveResult struct {
	ClerkID string `json:"clerk_id"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

func registerAdminUserControlTowerRoutes(admin *gin.RouterGroup, queries *db.Queries, pool *pgxpool.Pool) {
	admin.GET("/users/overview", func(c *gin.Context) {
		if authRole(c) != "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "only superadmin can access user overview"})
			return
		}

		overview, err := queries.AdminUsersOverview(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve user overview"})
			return
		}

		hospitals, err := queries.ListHospitalsForAdminUsers(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve hospitals"})
			return
		}
		if hospitals == nil {
			hospitals = []db.ListHospitalsForAdminUsersRow{}
		}

		c.JSON(http.StatusOK, adminUsersOverviewResponse{
			TotalUsers:                    overview.TotalUsers,
			ActiveUsers:                   overview.ActiveUsers,
			InactiveUsers:                 overview.InactiveUsers,
			UnassignedUsers:               overview.UnassignedUsers,
			AdminRoleMismatchUsers:        overview.AdminRoleMismatchUsers,
			InactiveWithMembershipUsers:   overview.InactiveWithMembershipUsers,
			AssignedInactiveHospitalUsers: overview.AssignedInactiveHospitalUsers,
			NeverLoggedInUsers:            overview.NeverLoggedInUsers,
			Hospitals:                     hospitals,
		})
	})

	admin.GET("/users/search", func(c *gin.Context) {
		if authRole(c) != "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "only superadmin can search users"})
			return
		}

		params := buildAdminUsersSearchParams(c)

		items, err := queries.SearchAdminUsers(c.Request.Context(), params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search users"})
			return
		}
		if items == nil {
			items = []db.SearchAdminUsersRow{}
		}

		total, err := queries.CountAdminUsersSearch(c.Request.Context(), db.CountAdminUsersSearchParams{
			Column1: params.Column1,
			Column2: params.Column2,
			Column3: params.Column3,
			Column4: params.Column4,
			Column5: params.Column5,
			Column6: params.Column6,
			Column7: params.Column7,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count users"})
			return
		}

		responseItems := make([]adminUsersSearchItem, 0, len(items))
		for _, item := range items {
			responseItems = append(responseItems, adminUsersSearchItem{
				ClerkID:                  item.ClerkID,
				Name:                     item.Name,
				Email:                    item.Email,
				Username:                 item.Username,
				FirstName:                item.FirstName,
				LastName:                 item.LastName,
				Role:                     item.Role,
				IsActive:                 item.IsActive,
				CreatedAt:                timestamptzRFC3339(item.CreatedAt),
				UpdatedAt:                timestamptzRFC3339(item.UpdatedAt),
				LastLoginAt:              timestamptzRFC3339(item.LastLoginAt),
				MembershipHospitalID:     item.MembershipHospitalID,
				MembershipHospitalSlug:   item.MembershipHospitalSlug,
				MembershipHospitalName:   item.MembershipHospitalName,
				MembershipHospitalActive: item.MembershipHospitalActive,
				MembershipRole:           item.MembershipRole,
				Anomalies:                anomalyListFromSearchRow(item),
			})
		}

		c.JSON(http.StatusOK, adminUsersSearchResponse{
			Items:  responseItems,
			Total:  total,
			Limit:  params.Limit,
			Offset: params.Offset,
		})
	})

	admin.GET("/users/:clerkID", func(c *gin.Context) {
		if authRole(c) != "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "only superadmin can access user details"})
			return
		}

		targetClerkID := strings.TrimSpace(c.Param("clerkID"))
		if targetClerkID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "target clerk id is required"})
			return
		}

		detail, err := queries.GetAdminUserDetail(c.Request.Context(), targetClerkID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "target user not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve user detail"})
			return
		}

		actions, err := queries.ListAdminUserActionsByTarget(c.Request.Context(), db.ListAdminUserActionsByTargetParams{
			TargetClerkID: targetClerkID,
			Limit:         25,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve user actions"})
			return
		}
		if actions == nil {
			actions = []db.AdminUserAction{}
		}
		recentActions := make([]adminUserActionSummary, 0, len(actions))
		for _, action := range actions {
			recentActions = append(recentActions, adminUserActionSummary{
				ID:           action.ID,
				ActorClerkID: action.ActorClerkID,
				ActionType:   action.ActionType,
				Reason:       action.Reason,
				Metadata:     decodeJSONMap(action.Metadata),
				CreatedAt:    timestamptzRFC3339(action.CreatedAt),
			})
		}

		c.JSON(http.StatusOK, adminUserDetailResponse{
			User: adminUsersSearchItem{
				ClerkID:                  detail.ClerkID,
				Name:                     detail.Name,
				Email:                    detail.Email,
				Username:                 detail.Username,
				FirstName:                detail.FirstName,
				LastName:                 detail.LastName,
				Role:                     detail.Role,
				IsActive:                 detail.IsActive,
				CreatedAt:                timestamptzRFC3339(detail.CreatedAt),
				UpdatedAt:                timestamptzRFC3339(detail.UpdatedAt),
				LastLoginAt:              timestamptzRFC3339(detail.LastLoginAt),
				MembershipHospitalID:     detail.MembershipHospitalID,
				MembershipHospitalSlug:   detail.MembershipHospitalSlug,
				MembershipHospitalName:   detail.MembershipHospitalName,
				MembershipHospitalActive: detail.MembershipHospitalActive,
				MembershipRole:           detail.MembershipRole,
				Anomalies:                anomalyListFromDetailRow(detail),
			},
			RecentActions: recentActions,
		})
	})

	admin.POST("/users/:clerkID/reassign-hospital", func(c *gin.Context) {
		if authRole(c) != "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "only superadmin can reassign users"})
			return
		}

		targetClerkID := strings.TrimSpace(c.Param("clerkID"))
		if targetClerkID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "target clerk id is required"})
			return
		}

		var payload adminUserReassignHospitalRequest
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}

		payload.HospitalSlug = strings.TrimSpace(payload.HospitalSlug)
		parsedRole, ok := parseMembershipRole(payload.MembershipRole)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "membership_role must be either admin or user"})
			return
		}
		payload.MembershipRole = parsedRole
		payload.Reason = strings.TrimSpace(payload.Reason)

		if payload.HospitalSlug == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "hospital_slug is required"})
			return
		}
		if payload.Reason == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "reason is required"})
			return
		}

		err := withAdminUserMutationTx(c, pool, queries, targetClerkID, func(qtx *db.Queries, target db.GetUserByClerkIDForUpdateRow, before adminMembershipSnapshot) error {
			hospital, err := qtx.GetHospitalBySlugForUpdate(c.Request.Context(), payload.HospitalSlug)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return adminMutationError{status: http.StatusNotFound, message: "hospital not found"}
				}
				return adminMutationError{status: http.StatusInternalServerError, message: "failed to load hospital"}
			}
			if !hospital.IsActive {
				return adminMutationError{status: http.StatusNotFound, message: "hospital is inactive"}
			}

			if err := qtx.UpsertHospitalUserMembership(c.Request.Context(), db.UpsertHospitalUserMembershipParams{
				HospitalID:       hospital.ID,
				UserClerkID:      target.ClerkID,
				MembershipRole:   payload.MembershipRole,
				InvitedByClerkID: toNullableText(authClerkID(c)),
			}); err != nil {
				return adminMutationError{status: http.StatusInternalServerError, message: "failed to reassign membership"}
			}

			afterMembership := adminMembershipSnapshot{
				HospitalID:       hospital.ID,
				HospitalSlug:     hospital.Slug,
				MembershipRole:   payload.MembershipRole,
				HospitalIsActive: true,
				Exists:           true,
			}
			state := deriveCanonicalUserState(target.Role, afterMembership)

			if err := qtx.SetUserRoleByClerkID(c.Request.Context(), db.SetUserRoleByClerkIDParams{
				ClerkID: target.ClerkID,
				Role:    state.Role,
			}); err != nil {
				return adminMutationError{status: http.StatusInternalServerError, message: "failed to update user role"}
			}
			if err := qtx.SetUserActiveByClerkID(c.Request.Context(), db.SetUserActiveByClerkIDParams{
				ClerkID:  target.ClerkID,
				IsActive: state.IsActive,
			}); err != nil {
				return adminMutationError{status: http.StatusInternalServerError, message: "failed to update user active state"}
			}

			if err := insertAdminUserAudit(c, qtx, target.ClerkID, "reassign_hospital", payload.Reason,
				buildUserAuditState(target.Role, target.IsActive, before),
				buildUserAuditState(state.Role, state.IsActive, afterMembership),
				map[string]any{"hospital_slug": hospital.Slug, "membership_role": payload.MembershipRole},
			); err != nil {
				return adminMutationError{status: http.StatusInternalServerError, message: "failed to write audit log"}
			}

			return nil
		})
		if err != nil {
			handleAdminMutationError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	admin.POST("/users/:clerkID/set-membership-role", func(c *gin.Context) {
		if authRole(c) != "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "only superadmin can set membership role"})
			return
		}

		targetClerkID := strings.TrimSpace(c.Param("clerkID"))
		if targetClerkID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "target clerk id is required"})
			return
		}

		var payload adminUserSetMembershipRoleRequest
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}

		parsedRole, ok := parseMembershipRole(payload.MembershipRole)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "membership_role must be either admin or user"})
			return
		}
		payload.MembershipRole = parsedRole
		payload.Reason = strings.TrimSpace(payload.Reason)
		if payload.Reason == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "reason is required"})
			return
		}

		err := withAdminUserMutationTx(c, pool, queries, targetClerkID, func(qtx *db.Queries, target db.GetUserByClerkIDForUpdateRow, before adminMembershipSnapshot) error {
			if !before.Exists || before.HospitalID <= 0 {
				return adminMutationError{status: http.StatusConflict, message: "target user does not have an active membership"}
			}
			if !before.HospitalIsActive {
				return adminMutationError{status: http.StatusConflict, message: "target membership is attached to an inactive hospital"}
			}

			if err := qtx.UpsertHospitalUserMembership(c.Request.Context(), db.UpsertHospitalUserMembershipParams{
				HospitalID:       before.HospitalID,
				UserClerkID:      target.ClerkID,
				MembershipRole:   payload.MembershipRole,
				InvitedByClerkID: toNullableText(authClerkID(c)),
			}); err != nil {
				return adminMutationError{status: http.StatusInternalServerError, message: "failed to update membership role"}
			}

			afterMembership := before
			afterMembership.MembershipRole = payload.MembershipRole
			state := deriveCanonicalUserState(target.Role, afterMembership)

			if err := qtx.SetUserRoleByClerkID(c.Request.Context(), db.SetUserRoleByClerkIDParams{ClerkID: target.ClerkID, Role: state.Role}); err != nil {
				return adminMutationError{status: http.StatusInternalServerError, message: "failed to update user role"}
			}
			if err := qtx.SetUserActiveByClerkID(c.Request.Context(), db.SetUserActiveByClerkIDParams{ClerkID: target.ClerkID, IsActive: state.IsActive}); err != nil {
				return adminMutationError{status: http.StatusInternalServerError, message: "failed to update user active state"}
			}

			if err := insertAdminUserAudit(c, qtx, target.ClerkID, "set_membership_role", payload.Reason,
				buildUserAuditState(target.Role, target.IsActive, before),
				buildUserAuditState(state.Role, state.IsActive, afterMembership),
				map[string]any{"membership_role": payload.MembershipRole},
			); err != nil {
				return adminMutationError{status: http.StatusInternalServerError, message: "failed to write audit log"}
			}

			return nil
		})
		if err != nil {
			handleAdminMutationError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	admin.POST("/users/:clerkID/detach-membership", func(c *gin.Context) {
		if authRole(c) != "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "only superadmin can detach membership"})
			return
		}

		targetClerkID := strings.TrimSpace(c.Param("clerkID"))
		if targetClerkID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "target clerk id is required"})
			return
		}

		var payload adminUserReasonRequest
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}
		payload.Reason = strings.TrimSpace(payload.Reason)
		if payload.Reason == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "reason is required"})
			return
		}

		err := withAdminUserMutationTx(c, pool, queries, targetClerkID, func(qtx *db.Queries, target db.GetUserByClerkIDForUpdateRow, before adminMembershipSnapshot) error {
			if !before.Exists || before.HospitalID <= 0 {
				return adminMutationError{status: http.StatusConflict, message: "target user does not have an active membership"}
			}

			affected, err := qtx.DeactivateHospitalMembershipByUser(c.Request.Context(), target.ClerkID)
			if err != nil {
				return adminMutationError{status: http.StatusInternalServerError, message: "failed to detach membership"}
			}
			if affected == 0 {
				return adminMutationError{status: http.StatusConflict, message: "target user does not have an active membership"}
			}

			afterMembership := adminMembershipSnapshot{}
			state := deriveCanonicalUserState(target.Role, afterMembership)
			if err := qtx.SetUserRoleByClerkID(c.Request.Context(), db.SetUserRoleByClerkIDParams{ClerkID: target.ClerkID, Role: state.Role}); err != nil {
				return adminMutationError{status: http.StatusInternalServerError, message: "failed to update user role"}
			}
			if err := qtx.SetUserActiveByClerkID(c.Request.Context(), db.SetUserActiveByClerkIDParams{ClerkID: target.ClerkID, IsActive: state.IsActive}); err != nil {
				return adminMutationError{status: http.StatusInternalServerError, message: "failed to update user active state"}
			}

			if err := insertAdminUserAudit(c, qtx, target.ClerkID, "detach_membership", payload.Reason,
				buildUserAuditState(target.Role, target.IsActive, before),
				buildUserAuditState(state.Role, state.IsActive, afterMembership),
				nil,
			); err != nil {
				return adminMutationError{status: http.StatusInternalServerError, message: "failed to write audit log"}
			}

			return nil
		})
		if err != nil {
			handleAdminMutationError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	admin.POST("/users/:clerkID/set-active", func(c *gin.Context) {
		if authRole(c) != "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "only superadmin can set user active state"})
			return
		}

		targetClerkID := strings.TrimSpace(c.Param("clerkID"))
		if targetClerkID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "target clerk id is required"})
			return
		}

		var payload adminUserSetActiveRequest
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}
		payload.Reason = strings.TrimSpace(payload.Reason)
		if payload.Reason == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "reason is required"})
			return
		}

		err := runAdminUserSetActiveMutation(c, pool, queries, targetClerkID, payload.IsActive, payload.Reason, "set_active")
		if err != nil {
			handleAdminMutationError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	admin.POST("/users/:clerkID/recompute-state", func(c *gin.Context) {
		if authRole(c) != "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "only superadmin can recompute user state"})
			return
		}

		targetClerkID := strings.TrimSpace(c.Param("clerkID"))
		if targetClerkID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "target clerk id is required"})
			return
		}

		var payload adminUserReasonRequest
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}
		payload.Reason = strings.TrimSpace(payload.Reason)
		if payload.Reason == "" {
			payload.Reason = "state recompute"
		}

		err := withAdminUserMutationTx(c, pool, queries, targetClerkID, func(qtx *db.Queries, target db.GetUserByClerkIDForUpdateRow, before adminMembershipSnapshot) error {
			membership := before
			if membership.Exists && !membership.HospitalIsActive {
				if _, err := qtx.DeactivateHospitalMembershipByUser(c.Request.Context(), target.ClerkID); err != nil {
					return adminMutationError{status: http.StatusInternalServerError, message: "failed to deactivate inactive hospital membership"}
				}
				membership = adminMembershipSnapshot{}
			}

			state := deriveCanonicalUserState(target.Role, membership)
			if err := qtx.SetUserRoleByClerkID(c.Request.Context(), db.SetUserRoleByClerkIDParams{ClerkID: target.ClerkID, Role: state.Role}); err != nil {
				return adminMutationError{status: http.StatusInternalServerError, message: "failed to update user role"}
			}
			if err := qtx.SetUserActiveByClerkID(c.Request.Context(), db.SetUserActiveByClerkIDParams{ClerkID: target.ClerkID, IsActive: state.IsActive}); err != nil {
				return adminMutationError{status: http.StatusInternalServerError, message: "failed to update user active state"}
			}

			if err := insertAdminUserAudit(c, qtx, target.ClerkID, "recompute_state", payload.Reason,
				buildUserAuditState(target.Role, target.IsActive, before),
				buildUserAuditState(state.Role, state.IsActive, membership),
				nil,
			); err != nil {
				return adminMutationError{status: http.StatusInternalServerError, message: "failed to write audit log"}
			}

			return nil
		})
		if err != nil {
			handleAdminMutationError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	admin.POST("/users/bulk/set-active", func(c *gin.Context) {
		if authRole(c) != "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "only superadmin can run bulk user actions"})
			return
		}

		var payload adminUserBulkSetActiveRequest
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}

		payload.Reason = strings.TrimSpace(payload.Reason)
		if payload.Reason == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "reason is required"})
			return
		}
		if len(payload.ClerkIDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "clerk_ids is required"})
			return
		}

		seen := make(map[string]struct{}, len(payload.ClerkIDs))
		results := make([]adminUserBulkSetActiveResult, 0, len(payload.ClerkIDs))
		for _, rawID := range payload.ClerkIDs {
			targetClerkID := strings.TrimSpace(rawID)
			if targetClerkID == "" {
				continue
			}
			if _, exists := seen[targetClerkID]; exists {
				continue
			}
			seen[targetClerkID] = struct{}{}

			err := runAdminUserSetActiveMutation(c, pool, queries, targetClerkID, payload.IsActive, payload.Reason, "bulk_set_active")
			if err != nil {
				results = append(results, adminUserBulkSetActiveResult{
					ClerkID: targetClerkID,
					OK:      false,
					Error:   adminMutationErrorMessage(err),
				})
				continue
			}

			results = append(results, adminUserBulkSetActiveResult{ClerkID: targetClerkID, OK: true})
		}

		c.JSON(http.StatusOK, gin.H{"results": results})
	})
}

type adminMutationError struct {
	status  int
	message string
}

func (e adminMutationError) Error() string {
	return e.message
}

func handleAdminMutationError(c *gin.Context, err error) {
	var mutationErr adminMutationError
	if errors.As(err, &mutationErr) {
		c.JSON(mutationErr.status, gin.H{"error": mutationErr.message})
		return
	}

	c.JSON(http.StatusInternalServerError, gin.H{"error": "request failed"})
}

func adminMutationErrorMessage(err error) string {
	var mutationErr adminMutationError
	if errors.As(err, &mutationErr) {
		return mutationErr.message
	}
	return "request failed"
}

func runAdminUserSetActiveMutation(
	c *gin.Context,
	pool *pgxpool.Pool,
	queries *db.Queries,
	targetClerkID string,
	desiredActive bool,
	reason string,
	actionType string,
) error {
	return withAdminUserMutationTx(c, pool, queries, targetClerkID, func(qtx *db.Queries, target db.GetUserByClerkIDForUpdateRow, before adminMembershipSnapshot) error {
		afterMembership := before
		afterRole := target.Role
		afterIsActive := target.IsActive

		if desiredActive {
			if !before.Exists || before.HospitalID <= 0 {
				return adminMutationError{status: http.StatusConflict, message: "cannot activate user without active hospital membership"}
			}
			if !before.HospitalIsActive {
				return adminMutationError{status: http.StatusConflict, message: "cannot activate user in inactive hospital"}
			}
			state := deriveCanonicalUserState(target.Role, before)
			afterRole = state.Role
			afterIsActive = state.IsActive
		} else {
			if _, err := qtx.DeactivateHospitalMembershipByUser(c.Request.Context(), target.ClerkID); err != nil {
				return adminMutationError{status: http.StatusInternalServerError, message: "failed to deactivate membership"}
			}
			afterMembership = adminMembershipSnapshot{}
			afterRole = "user"
			afterIsActive = false
		}

		if err := qtx.SetUserRoleByClerkID(c.Request.Context(), db.SetUserRoleByClerkIDParams{ClerkID: target.ClerkID, Role: afterRole}); err != nil {
			return adminMutationError{status: http.StatusInternalServerError, message: "failed to update user role"}
		}
		if err := qtx.SetUserActiveByClerkID(c.Request.Context(), db.SetUserActiveByClerkIDParams{ClerkID: target.ClerkID, IsActive: afterIsActive}); err != nil {
			return adminMutationError{status: http.StatusInternalServerError, message: "failed to update user active state"}
		}

		if err := insertAdminUserAudit(c, qtx, target.ClerkID, actionType, reason,
			buildUserAuditState(target.Role, target.IsActive, before),
			buildUserAuditState(afterRole, afterIsActive, afterMembership),
			map[string]any{"desired_active": desiredActive},
		); err != nil {
			return adminMutationError{status: http.StatusInternalServerError, message: "failed to write audit log"}
		}

		return nil
	})
}

func withAdminUserMutationTx(
	c *gin.Context,
	pool *pgxpool.Pool,
	queries *db.Queries,
	targetClerkID string,
	fn func(qtx *db.Queries, target db.GetUserByClerkIDForUpdateRow, before adminMembershipSnapshot) error,
) error {
	tx, err := pool.Begin(c.Request.Context())
	if err != nil {
		return adminMutationError{status: http.StatusInternalServerError, message: "failed to start transaction"}
	}
	defer tx.Rollback(c.Request.Context())

	qtx := queries.WithTx(tx)
	target, err := qtx.GetUserByClerkIDForUpdate(c.Request.Context(), targetClerkID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return adminMutationError{status: http.StatusNotFound, message: "target user not found"}
		}
		return adminMutationError{status: http.StatusInternalServerError, message: "failed to load target user"}
	}
	if target.DeletedAt.Valid {
		return adminMutationError{status: http.StatusNotFound, message: "target user is deleted"}
	}
	if target.Role == "superadmin" {
		return adminMutationError{status: http.StatusBadRequest, message: "superadmin target is not allowed for this action"}
	}

	beforeMembership, err := loadAdminMembershipSnapshot(c, qtx, target.ClerkID)
	if err != nil {
		return err
	}

	if err := fn(qtx, target, beforeMembership); err != nil {
		return err
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		return adminMutationError{status: http.StatusInternalServerError, message: "failed to commit mutation"}
	}

	return nil
}

func loadAdminMembershipSnapshot(c *gin.Context, qtx *db.Queries, targetClerkID string) (adminMembershipSnapshot, error) {
	membership, err := qtx.GetActiveHospitalMembershipContextByUser(c.Request.Context(), targetClerkID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return adminMembershipSnapshot{}, nil
		}
		return adminMembershipSnapshot{}, adminMutationError{status: http.StatusInternalServerError, message: "failed to load membership context"}
	}

	return adminMembershipSnapshot{
		HospitalID:       membership.HospitalID,
		HospitalSlug:     membership.HospitalSlug,
		MembershipRole:   membership.MembershipRole,
		HospitalIsActive: membership.HospitalIsActive,
		Exists:           membership.HospitalID > 0,
	}, nil
}

func buildAdminUsersSearchParams(c *gin.Context) db.SearchAdminUsersParams {
	q := strings.TrimSpace(c.Query("q"))
	role := strings.TrimSpace(c.Query("role"))
	active := strings.TrimSpace(c.Query("is_active"))
	hospitalSlug := strings.TrimSpace(c.Query("hospital_slug"))
	membershipRole := strings.TrimSpace(c.Query("membership_role"))
	anomaly := strings.TrimSpace(c.Query("anomaly"))
	lastLoginBucket := strings.TrimSpace(c.Query("last_login_bucket"))

	limit := adminUsersDefaultPageSize
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err == nil {
			limit = parsedLimit
		}
	}
	if limit <= 0 {
		limit = adminUsersDefaultPageSize
	}
	if limit > adminUsersMaxPageSize {
		limit = adminUsersMaxPageSize
	}

	offset := 0
	if rawOffset := strings.TrimSpace(c.Query("offset")); rawOffset != "" {
		parsedOffset, err := strconv.Atoi(rawOffset)
		if err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	return db.SearchAdminUsersParams{
		Column1: q,
		Column2: role,
		Column3: active,
		Column4: hospitalSlug,
		Column5: membershipRole,
		Column6: anomaly,
		Column7: lastLoginBucket,
		Limit:   int32(limit),
		Offset:  int32(offset),
	}
}

func buildUserAuditState(role string, isActive bool, membership adminMembershipSnapshot) map[string]any {
	return map[string]any{
		"role":                       role,
		"is_active":                  isActive,
		"membership_hospital_id":     membership.HospitalID,
		"membership_hospital_slug":   membership.HospitalSlug,
		"membership_role":            membership.MembershipRole,
		"membership_hospital_active": membership.HospitalIsActive,
		"has_membership":             membership.Exists,
	}
}

func insertAdminUserAudit(
	c *gin.Context,
	qtx *db.Queries,
	targetClerkID string,
	actionType string,
	reason string,
	before map[string]any,
	after map[string]any,
	metadata map[string]any,
) error {
	beforeBytes, err := json.Marshal(before)
	if err != nil {
		return fmt.Errorf("marshal before state: %w", err)
	}
	afterBytes, err := json.Marshal(after)
	if err != nil {
		return fmt.Errorf("marshal after state: %w", err)
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	return qtx.InsertAdminUserAction(c.Request.Context(), db.InsertAdminUserActionParams{
		ActorClerkID:  authClerkID(c),
		TargetClerkID: targetClerkID,
		ActionType:    actionType,
		Reason:        reason,
		BeforeState:   beforeBytes,
		AfterState:    afterBytes,
		Metadata:      metadataBytes,
	})
}

func anomalyListFromSearchRow(item db.SearchAdminUsersRow) []string {
	return anomalyList(
		item.AnomalyNoMembership,
		item.AnomalyAdminRoleMismatch,
		item.AnomalyInactiveWithMembership,
		item.AnomalyAssignedInactiveHospital,
		item.AnomalyNeverLoggedIn,
	)
}

func anomalyListFromDetailRow(item db.GetAdminUserDetailRow) []string {
	return anomalyList(
		item.AnomalyNoMembership,
		item.AnomalyAdminRoleMismatch,
		item.AnomalyInactiveWithMembership,
		item.AnomalyAssignedInactiveHospital,
		item.AnomalyNeverLoggedIn,
	)
}

func anomalyList(noMembership, adminMismatch, inactiveWithMembership, assignedInactiveHospital, neverLoggedIn bool) []string {
	items := make([]string, 0, 5)
	if noMembership {
		items = append(items, adminUserAnomalyNoMembership)
	}
	if adminMismatch {
		items = append(items, adminUserAnomalyAdminRoleMismatch)
	}
	if inactiveWithMembership {
		items = append(items, adminUserAnomalyInactiveWithMembership)
	}
	if assignedInactiveHospital {
		items = append(items, adminUserAnomalyAssignedInactiveHospital)
	}
	if neverLoggedIn {
		items = append(items, adminUserAnomalyNeverLoggedIn)
	}
	return items
}

func decodeJSONMap(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return map[string]any{}
	}
	return decoded
}

func timestamptzRFC3339(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.UTC().Format(time.RFC3339)
}
