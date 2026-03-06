package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/sheeld/sheeld/internal/db/generated"
)

// TokenClaims holds the JWT payload.
type TokenClaims struct {
	UserID uuid.UUID `json:"user_id"`
	OrgID  uuid.UUID `json:"org_id"`
	jwt.RegisteredClaims
}

// RegisterResult is returned after successful registration.
type RegisterResult struct {
	Organization generated.Organization `json:"organization"`
	User         generated.User         `json:"user"`
	Token        string                 `json:"token"`
}

// LoginResult is returned after successful login.
type LoginResult struct {
	User  generated.User `json:"user"`
	Token string         `json:"token"`
}

// CreateAPIKeyResult is returned after creating an API key.
type CreateAPIKeyResult struct {
	APIKey generated.ApiKey `json:"api_key"`
	RawKey string           `json:"raw_key"` // Only returned once at creation time
}

// AuthService handles authentication and authorization.
type AuthService struct {
	queries       *generated.Queries
	jwtSecret     []byte
	jwtExpiration time.Duration
}

// NewAuthService creates a new AuthService.
func NewAuthService(queries *generated.Queries, jwtSecret string, jwtExpiration time.Duration) *AuthService {
	return &AuthService{
		queries:       queries,
		jwtSecret:     []byte(jwtSecret),
		jwtExpiration: jwtExpiration,
	}
}

// Register creates a new organization and user.
func (s *AuthService) Register(ctx context.Context, orgName, email, password string) (*RegisterResult, error) {
	// Create organization
	org, err := s.queries.CreateOrganization(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("creating organization: %w", err)
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	// Create user
	user, err := s.queries.CreateUser(ctx, generated.CreateUserParams{
		OrganizationID: org.ID,
		Email:          email,
		PasswordHash:   string(hash),
	})
	if err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}

	// Generate JWT
	token, err := s.generateToken(user.ID, org.ID)
	if err != nil {
		return nil, fmt.Errorf("generating token: %w", err)
	}

	return &RegisterResult{
		Organization: org,
		User:         user,
		Token:        token,
	}, nil
}

// Login authenticates a user and returns a JWT.
func (s *AuthService) Login(ctx context.Context, email, password string) (*LoginResult, error) {
	user, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	token, err := s.generateToken(user.ID, user.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("generating token: %w", err)
	}

	return &LoginResult{
		User:  user,
		Token: token,
	}, nil
}

// CreateAPIKey generates a new API key for the organization.
func (s *AuthService) CreateAPIKey(ctx context.Context, orgID uuid.UUID, name string) (*CreateAPIKeyResult, error) {
	// Generate random key (32 bytes = 64 hex chars)
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return nil, fmt.Errorf("generating random key: %w", err)
	}
	rawKey := "shld_" + hex.EncodeToString(rawBytes)

	// Hash the key for storage
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])

	// Store first 8 chars as prefix for identification
	keyPrefix := rawKey[:13] // "shld_" + 8 hex chars

	apiKey, err := s.queries.CreateAPIKey(ctx, generated.CreateAPIKeyParams{
		OrganizationID: orgID,
		Name:           name,
		KeyHash:        keyHash,
		KeyPrefix:      keyPrefix,
	})
	if err != nil {
		return nil, fmt.Errorf("creating API key: %w", err)
	}

	return &CreateAPIKeyResult{
		APIKey: apiKey,
		RawKey: rawKey,
	}, nil
}

// ListAPIKeys returns all API keys for an organization.
func (s *AuthService) ListAPIKeys(ctx context.Context, orgID uuid.UUID) ([]generated.ApiKey, error) {
	return s.queries.ListAPIKeysByOrganization(ctx, orgID)
}

// RevokeAPIKey revokes an API key.
func (s *AuthService) RevokeAPIKey(ctx context.Context, orgID uuid.UUID, keyID uuid.UUID) error {
	return s.queries.RevokeAPIKey(ctx, generated.RevokeAPIKeyParams{
		ID:             keyID,
		OrganizationID: orgID,
	})
}

// ValidateToken validates a JWT and returns the claims.
func (s *AuthService) ValidateToken(tokenString string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*TokenClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// ValidateAPIKey validates an API key and returns the organization ID.
func (s *AuthService) ValidateAPIKey(ctx context.Context, rawKey string) (uuid.UUID, error) {
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])

	apiKey, err := s.queries.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid API key")
	}

	return apiKey.OrganizationID, nil
}

// RefreshToken generates a new JWT for the given claims.
func (s *AuthService) RefreshToken(claims *TokenClaims) (string, error) {
	return s.generateToken(claims.UserID, claims.OrgID)
}

func (s *AuthService) generateToken(userID, orgID uuid.UUID) (string, error) {
	claims := &TokenClaims{
		UserID: userID,
		OrgID:  orgID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.jwtExpiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "sheeld",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}
