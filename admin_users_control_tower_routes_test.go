package main

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBuildAdminUsersSearchParams_Defaults(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest("GET", "/admin/users/search", nil)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request

	params := buildAdminUsersSearchParams(context)

	if params.Limit != adminUsersDefaultPageSize {
		t.Fatalf("expected default limit %d, got %d", adminUsersDefaultPageSize, params.Limit)
	}
	if params.Offset != 0 {
		t.Fatalf("expected default offset 0, got %d", params.Offset)
	}
	if params.Column1 != "" || params.Column2 != "" || params.Column3 != "" {
		t.Fatalf("expected empty filters by default")
	}
}

func TestBuildAdminUsersSearchParams_WithValues(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	query := url.Values{}
	query.Set("q", "alice")
	query.Set("role", "admin")
	query.Set("is_active", "active")
	query.Set("hospital_slug", "north-hospital")
	query.Set("membership_role", "admin")
	query.Set("anomaly", "no_membership")
	query.Set("last_login_bucket", "gt_90d")
	query.Set("limit", "500")
	query.Set("offset", "25")

	request := httptest.NewRequest("GET", "/admin/users/search?"+query.Encode(), nil)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request

	params := buildAdminUsersSearchParams(context)

	if params.Column1 != "alice" || params.Column2 != "admin" || params.Column3 != "active" {
		t.Fatalf("unexpected text filters: %#v", params)
	}
	if params.Column4 != "north-hospital" || params.Column5 != "admin" {
		t.Fatalf("unexpected membership filters: %#v", params)
	}
	if params.Column6 != "no_membership" || params.Column7 != "gt_90d" {
		t.Fatalf("unexpected anomaly/login filters: %#v", params)
	}
	if params.Limit != adminUsersMaxPageSize {
		t.Fatalf("expected clamped limit %d, got %d", adminUsersMaxPageSize, params.Limit)
	}
	if params.Offset != 25 {
		t.Fatalf("expected offset 25, got %d", params.Offset)
	}
}

func TestAnomalyList(t *testing.T) {
	t.Parallel()

	all := anomalyList(true, true, true, true, true)
	if len(all) != 5 {
		t.Fatalf("expected 5 anomalies, got %d", len(all))
	}

	none := anomalyList(false, false, false, false, false)
	if len(none) != 0 {
		t.Fatalf("expected no anomalies, got %d", len(none))
	}
}
