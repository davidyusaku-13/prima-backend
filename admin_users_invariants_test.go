package main

import "testing"

func TestDeriveCanonicalUserState(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		globalRole string
		membership adminMembershipSnapshot
		expected   canonicalUserState
	}{
		{
			name:       "superadmin always active",
			globalRole: "superadmin",
			membership: adminMembershipSnapshot{},
			expected:   canonicalUserState{Role: "superadmin", IsActive: true},
		},
		{
			name:       "admin membership becomes admin",
			globalRole: "user",
			membership: adminMembershipSnapshot{Exists: true, HospitalID: 9, MembershipRole: "admin", HospitalIsActive: true},
			expected:   canonicalUserState{Role: "admin", IsActive: true},
		},
		{
			name:       "member role becomes active user",
			globalRole: "admin",
			membership: adminMembershipSnapshot{Exists: true, HospitalID: 10, MembershipRole: "user", HospitalIsActive: true},
			expected:   canonicalUserState{Role: "user", IsActive: true},
		},
		{
			name:       "missing membership becomes inactive user",
			globalRole: "admin",
			membership: adminMembershipSnapshot{},
			expected:   canonicalUserState{Role: "user", IsActive: false},
		},
		{
			name:       "inactive hospital membership becomes inactive user",
			globalRole: "user",
			membership: adminMembershipSnapshot{Exists: true, HospitalID: 11, MembershipRole: "admin", HospitalIsActive: false},
			expected:   canonicalUserState{Role: "user", IsActive: false},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := deriveCanonicalUserState(tc.globalRole, tc.membership)
			if got != tc.expected {
				t.Fatalf("expected %#v, got %#v", tc.expected, got)
			}
		})
	}
}

func TestDeriveAdminUserAnomalies(t *testing.T) {
	t.Parallel()

	anomalies := deriveAdminUserAnomalies("admin", true, false, "", false, false)
	if len(anomalies) < 3 {
		t.Fatalf("expected multiple anomalies, got %#v", anomalies)
	}

	anomalies = deriveAdminUserAnomalies("user", true, true, "user", true, true)
	if len(anomalies) != 0 {
		t.Fatalf("expected no anomalies, got %#v", anomalies)
	}
}
