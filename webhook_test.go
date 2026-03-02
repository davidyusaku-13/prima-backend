package main

import (
	"context"
	"testing"

	"backend/internal/db"
)

type webhookQueriesStub struct {
	upsertCalls      int
	softDeleteCalls  int
	updatedLastLogin []string
}

func (s *webhookQueriesStub) UpsertUserWithRole(_ context.Context, _ db.UpsertUserWithRoleParams) error {
	s.upsertCalls++
	return nil
}

func (s *webhookQueriesStub) SoftDeleteUserByClerkID(_ context.Context, _ string) error {
	s.softDeleteCalls++
	return nil
}

func (s *webhookQueriesStub) UpdateLastLogin(_ context.Context, clerkID string) error {
	s.updatedLastLogin = append(s.updatedLastLogin, clerkID)
	return nil
}

func TestHandleClerkWebhookEvent_SessionCreatedUpdatesLastLogin(t *testing.T) {
	t.Parallel()

	queries := &webhookQueriesStub{}
	event := ClerkUserWebhookEvent{Type: "session.created"}
	event.Data.UserID = "user_123"

	result, err := handleClerkWebhookEvent(context.Background(), queries, event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Ignored != "" {
		t.Fatalf("expected no ignored reason, got %q", result.Ignored)
	}
	if len(queries.updatedLastLogin) != 1 {
		t.Fatalf("expected one UpdateLastLogin call, got %d", len(queries.updatedLastLogin))
	}
	if queries.updatedLastLogin[0] != "user_123" {
		t.Fatalf("expected update for user_123, got %q", queries.updatedLastLogin[0])
	}
}

func TestHandleClerkWebhookEvent_SessionCreatedMissingUserIDIgnored(t *testing.T) {
	t.Parallel()

	queries := &webhookQueriesStub{}
	event := ClerkUserWebhookEvent{Type: "session.created"}

	result, err := handleClerkWebhookEvent(context.Background(), queries, event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Ignored == "" {
		t.Fatal("expected ignored reason for missing user_id")
	}
	if len(queries.updatedLastLogin) != 0 {
		t.Fatalf("expected no UpdateLastLogin calls, got %d", len(queries.updatedLastLogin))
	}
}

func TestHandleClerkWebhookEvent_UserCreatedStillUpserts(t *testing.T) {
	t.Parallel()

	queries := &webhookQueriesStub{}
	event := ClerkUserWebhookEvent{Type: "user.created"}
	event.Data.ID = "user_999"
	event.Data.FirstName = "Jane"
	event.Data.LastName = "Doe"
	event.Data.Username = "janedoe"
	event.Data.EmailAddresses = []struct {
		ID           string `json:"id"`
		EmailAddress string `json:"email_address"`
	}{
		{ID: "e_1", EmailAddress: "jane@example.com"},
	}
	event.Data.PrimaryEmailAddressID = "e_1"

	result, err := handleClerkWebhookEvent(context.Background(), queries, event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Ignored != "" {
		t.Fatalf("expected no ignored reason, got %q", result.Ignored)
	}
	if queries.upsertCalls != 1 {
		t.Fatalf("expected one upsert call, got %d", queries.upsertCalls)
	}
	if len(queries.updatedLastLogin) != 0 {
		t.Fatalf("expected no UpdateLastLogin calls, got %d", len(queries.updatedLastLogin))
	}
}

func TestHandleClerkWebhookEvent_UserUpdatedStillUpserts(t *testing.T) {
	t.Parallel()

	queries := &webhookQueriesStub{}
	event := ClerkUserWebhookEvent{Type: "user.updated"}
	event.Data.ID = "user_111"
	event.Data.FirstName = "Alex"

	result, err := handleClerkWebhookEvent(context.Background(), queries, event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Ignored != "" {
		t.Fatalf("expected no ignored reason, got %q", result.Ignored)
	}
	if queries.upsertCalls != 1 {
		t.Fatalf("expected one upsert call, got %d", queries.upsertCalls)
	}
	if len(queries.updatedLastLogin) != 0 {
		t.Fatalf("expected no UpdateLastLogin calls, got %d", len(queries.updatedLastLogin))
	}
}

func TestHandleClerkWebhookEvent_UserDeletedStillSoftDeletes(t *testing.T) {
	t.Parallel()

	queries := &webhookQueriesStub{}
	event := ClerkUserWebhookEvent{Type: "user.deleted"}
	event.Data.ID = "user_222"

	result, err := handleClerkWebhookEvent(context.Background(), queries, event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Ignored != "" {
		t.Fatalf("expected no ignored reason, got %q", result.Ignored)
	}
	if queries.softDeleteCalls != 1 {
		t.Fatalf("expected one soft delete call, got %d", queries.softDeleteCalls)
	}
	if len(queries.updatedLastLogin) != 0 {
		t.Fatalf("expected no UpdateLastLogin calls, got %d", len(queries.updatedLastLogin))
	}
}

func TestHandleClerkWebhookEvent_UserDeletedMissingIDRemainsNoOp(t *testing.T) {
	t.Parallel()

	queries := &webhookQueriesStub{}
	event := ClerkUserWebhookEvent{Type: "user.deleted"}

	result, err := handleClerkWebhookEvent(context.Background(), queries, event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Ignored != "" {
		t.Fatalf("expected empty ignored reason to preserve behavior, got %q", result.Ignored)
	}
	if queries.softDeleteCalls != 0 {
		t.Fatalf("expected no soft delete calls, got %d", queries.softDeleteCalls)
	}
	if len(queries.updatedLastLogin) != 0 {
		t.Fatalf("expected no UpdateLastLogin calls, got %d", len(queries.updatedLastLogin))
	}
}
