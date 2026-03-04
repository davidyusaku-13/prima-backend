package main

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	clerk "github.com/clerk/clerk-sdk-go/v2"
	clerkjwt "github.com/clerk/clerk-sdk-go/v2/jwt"
	"github.com/gin-gonic/gin"
)

func TestSetClerkAuthorizedPartiesFromCSV(t *testing.T) {
	if err := setClerkAuthorizedPartiesFromCSV("https://app.example.com, https://admin.example.com"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !isAuthorizedParty("https://app.example.com") {
		t.Fatal("expected app origin to be authorized")
	}
	if isAuthorizedParty("https://evil.example.com") {
		t.Fatal("expected unknown origin to be rejected")
	}
}

func TestVerifyClerkIdentityFromRequest_UsesAuthorizedPartyHandler(t *testing.T) {
	if err := setClerkAuthorizedPartiesFromCSV("https://app.example.com"); err != nil {
		t.Fatalf("failed to configure authorized parties: %v", err)
	}

	oldVerify := verifyClerkToken
	t.Cleanup(func() {
		verifyClerkToken = oldVerify
	})

	verifyClerkToken = func(_ context.Context, params *clerkjwt.VerifyParams) (*clerk.SessionClaims, error) {
		if params.AuthorizedPartyHandler == nil {
			t.Fatal("expected AuthorizedPartyHandler to be set")
		}
		if !params.AuthorizedPartyHandler("https://app.example.com") {
			t.Fatal("expected configured authorized party to pass")
		}
		if params.AuthorizedPartyHandler("https://evil.example.com") {
			t.Fatal("expected unauthorized party to fail")
		}
		return &clerk.SessionClaims{RegisteredClaims: clerk.RegisteredClaims{Subject: "user_123"}}, nil
	}

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest("GET", "/admin", nil)
	req.Header.Set("Authorization", "Bearer token_123")
	ctx.Request = req

	identity, err := verifyClerkIdentityFromRequest(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if identity.ClerkID != "user_123" {
		t.Fatalf("expected user_123, got %q", identity.ClerkID)
	}
}

func TestVerifyClerkIdentityFromRequest_RejectsWhenAuthorizedPartyConfigMissing(t *testing.T) {
	oldVerify := verifyClerkToken
	t.Cleanup(func() {
		verifyClerkToken = oldVerify
	})
	verifyClerkToken = func(_ context.Context, _ *clerkjwt.VerifyParams) (*clerk.SessionClaims, error) {
		return nil, errors.New("should not be called")
	}
	clerkAuthorizedPartiesMu.Lock()
	clerkAuthorizedParties = map[string]struct{}{}
	clerkAuthorizedPartiesMu.Unlock()

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest("GET", "/admin", nil)
	req.Header.Set("Authorization", "Bearer token_123")
	ctx.Request = req

	_, err := verifyClerkIdentityFromRequest(ctx)
	if err == nil {
		t.Fatal("expected misconfiguration error")
	}
}
