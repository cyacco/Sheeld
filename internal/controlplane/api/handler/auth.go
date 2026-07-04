package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/shared/middleware"
	"github.com/sheeld/sheeld/internal/shared/response"
	"github.com/sheeld/sheeld/internal/controlplane/service"
)

// AuthHandler handles auth-related HTTP requests.
type AuthHandler struct {
	authService *service.AuthService
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

type registerRequest struct {
	OrgName  string `json:"org_name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type createAPIKeyRequest struct {
	Name string `json:"name"`
}

// Register handles POST /v1/auth/register.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.OrgName == "" {
		response.ValidationError(w, "org_name", "organization name is required")
		return
	}
	if req.Email == "" {
		response.ValidationError(w, "email", "email is required")
		return
	}
	if len(req.Password) < 8 {
		response.ValidationError(w, "password", "password must be at least 8 characters")
		return
	}

	result, err := h.authService.Register(r.Context(), req.OrgName, req.Email, req.Password)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to register")
		return
	}

	response.JSON(w, http.StatusCreated, result)
}

// Login handles POST /v1/auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		response.Error(w, http.StatusBadRequest, "email and password are required")
		return
	}

	result, err := h.authService.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		response.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	response.JSON(w, http.StatusOK, result)
}

// CreateAPIKey handles POST /v1/auth/api-keys.
func (h *AuthHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())
	if orgID == uuid.Nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		response.ValidationError(w, "name", "API key name is required")
		return
	}

	result, err := h.authService.CreateAPIKey(r.Context(), orgID, req.Name)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to create API key")
		return
	}

	response.JSON(w, http.StatusCreated, result)
}

// ListAPIKeys handles GET /v1/auth/api-keys.
func (h *AuthHandler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())
	if orgID == uuid.Nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	keys, err := h.authService.ListAPIKeys(r.Context(), orgID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to list API keys")
		return
	}

	response.JSON(w, http.StatusOK, keys)
}

// Refresh handles POST /v1/auth/refresh.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	orgID := middleware.OrgIDFromContext(r.Context())
	if userID == uuid.Nil || orgID == uuid.Nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	claims := &service.TokenClaims{
		UserID: userID,
		OrgID:  orgID,
	}

	token, err := h.authService.RefreshToken(claims)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to refresh token")
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"token": token})
}

// RevokeAPIKey handles DELETE /v1/auth/api-keys/:id.
func (h *AuthHandler) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())
	if orgID == uuid.Nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	keyID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid API key ID")
		return
	}

	if err := h.authService.RevokeAPIKey(r.Context(), orgID, keyID); err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to revoke API key")
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
