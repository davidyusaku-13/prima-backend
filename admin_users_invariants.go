package main

import "strings"

const (
	adminUserAnomalyNoMembership             = "no_membership"
	adminUserAnomalyAdminRoleMismatch        = "admin_role_mismatch"
	adminUserAnomalyInactiveWithMembership   = "inactive_with_membership"
	adminUserAnomalyAssignedInactiveHospital = "assigned_inactive_hospital"
	adminUserAnomalyNeverLoggedIn            = "never_logged_in"
)

type adminMembershipSnapshot struct {
	HospitalID       int64
	HospitalSlug     string
	MembershipRole   string
	HospitalIsActive bool
	Exists           bool
}

type canonicalUserState struct {
	Role     string
	IsActive bool
}

func normalizeMembershipRole(role string) string {
	role = strings.TrimSpace(strings.ToLower(role))
	if role == "admin" {
		return "admin"
	}
	return "user"
}

func parseMembershipRole(role string) (string, bool) {
	role = strings.TrimSpace(strings.ToLower(role))
	if role == "admin" || role == "user" {
		return role, true
	}
	return "", false
}

func deriveCanonicalUserState(globalRole string, membership adminMembershipSnapshot) canonicalUserState {
	if globalRole == "superadmin" {
		return canonicalUserState{Role: "superadmin", IsActive: true}
	}

	if !membership.Exists || membership.HospitalID <= 0 || !membership.HospitalIsActive {
		return canonicalUserState{Role: "user", IsActive: false}
	}

	if normalizeMembershipRole(membership.MembershipRole) == "admin" {
		return canonicalUserState{Role: "admin", IsActive: true}
	}

	return canonicalUserState{Role: "user", IsActive: true}
}

func deriveAdminUserAnomalies(
	globalRole string,
	isActive bool,
	hasMembership bool,
	membershipRole string,
	membershipHospitalActive bool,
	hasLoggedIn bool,
) []string {
	anomalies := make([]string, 0, 5)

	if globalRole != "superadmin" && !hasMembership {
		anomalies = append(anomalies, adminUserAnomalyNoMembership)
	}

	if globalRole == "admin" && normalizeMembershipRole(membershipRole) != "admin" {
		anomalies = append(anomalies, adminUserAnomalyAdminRoleMismatch)
	}

	if !isActive && hasMembership {
		anomalies = append(anomalies, adminUserAnomalyInactiveWithMembership)
	}

	if hasMembership && !membershipHospitalActive {
		anomalies = append(anomalies, adminUserAnomalyAssignedInactiveHospital)
	}

	if !hasLoggedIn {
		anomalies = append(anomalies, adminUserAnomalyNeverLoggedIn)
	}

	return anomalies
}
