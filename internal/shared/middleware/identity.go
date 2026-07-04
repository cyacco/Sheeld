package middleware

import (
	"context"

	"github.com/google/uuid"
)

type authContextKey string

const (
	// UserIDKey stores the authenticated user's ID in context.
	UserIDKey authContextKey = "user_id"
	// OrgIDKey stores the authenticated user's organization ID in context.
	OrgIDKey authContextKey = "org_id"
)

// OrgIDFromContext extracts the organization ID from the request context.
func OrgIDFromContext(ctx context.Context) uuid.UUID {
	if id, ok := ctx.Value(OrgIDKey).(uuid.UUID); ok {
		return id
	}
	return uuid.Nil
}

// UserIDFromContext extracts the user ID from the request context.
func UserIDFromContext(ctx context.Context) uuid.UUID {
	if id, ok := ctx.Value(UserIDKey).(uuid.UUID); ok {
		return id
	}
	return uuid.Nil
}
