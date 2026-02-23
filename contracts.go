package main

// APIUserContract is kept in sync with db.ListUsersRow as an API contract
// reference. The /users handler serializes db.ListUsersRow directly; this
// struct is not used for actual responses.
type APIUserContract struct {
	ClerkID     string `json:"clerk_id,omitempty"`
	Name        string `json:"name"`
	FirstName   string `json:"first_name,omitempty"`
	LastName    string `json:"last_name,omitempty"`
	Email       string `json:"email,omitempty"`
	Username    string `json:"username,omitempty"`
	Role        string `json:"role"`
	IsActive    bool   `json:"is_active"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	DeletedAt   string `json:"deleted_at,omitempty"`
	LastLoginAt string `json:"last_login_at,omitempty"`
}
