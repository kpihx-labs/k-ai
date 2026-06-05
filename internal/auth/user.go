package auth

import (
	"encoding/json"
	"net/http"

	"github.com/kpihx-labs/k-ai/internal/store"
	"golang.org/x/crypto/bcrypt"
)

type UserHandler struct {
	Store               *store.Store
	JWT                 *JWTManager
	RegistrationEnabled func() bool
}

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type tokenResponse struct {
	Token string         `json:"token"`
	User  userResponse   `json:"user"`
}

type userResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email,omitempty"`
	Role     string `json:"role"`
}

func (h *UserHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/register", h.register)
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("GET /auth/me", h.me)
}

func (h *UserHandler) register(w http.ResponseWriter, r *http.Request) {
	if !h.RegistrationEnabled() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "registration is disabled"})
		return
	}

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}
	if len(req.Password) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 8 characters"})
		return
	}

	existing, err := h.Store.GetUserByUsername(r.Context(), req.Username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if existing != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "username already taken"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	user, err := h.Store.CreateUser(r.Context(), req.Username, string(hash), req.Email, "user")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create user"})
		return
	}

	apiKey, _ := h.Store.CreateAPIKeyForUser(r.Context(), user.ID, user.Username+"-default", []string{"chat", "models"})

	token, err := h.JWT.Generate(user.ID, user.Username, user.Role)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not generate token"})
		return
	}

	resp := map[string]any{
		"token": token,
		"user":  userResponse{ID: user.ID, Username: user.Username, Email: user.Email, Role: user.Role},
	}
	if apiKey != nil {
		resp["api_key"] = apiKey.Key
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *UserHandler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}

	user, err := h.Store.GetUserByUsername(r.Context(), req.Username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	if !user.Enabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "account disabled"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	token, err := h.JWT.Generate(user.ID, user.Username, user.Role)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not generate token"})
		return
	}

	writeJSON(w, http.StatusOK, tokenResponse{
		Token: token,
		User:  userResponse{ID: user.ID, Username: user.Username, Email: user.Email, Role: user.Role},
	})
}

func (h *UserHandler) me(w http.ResponseWriter, r *http.Request) {
	tokenStr := bearerToken(r)
	if tokenStr == "" {
		writeUnauthorized(w, "missing bearer token")
		return
	}

	claims, err := h.JWT.Validate(tokenStr)
	if err != nil {
		writeUnauthorized(w, "invalid or expired token")
		return
	}

	user, err := h.Store.GetUserByID(r.Context(), claims.UserID)
	if err != nil || user == nil {
		writeUnauthorized(w, "user not found")
		return
	}
	if !user.Enabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "account disabled"})
		return
	}

	writeJSON(w, http.StatusOK, userResponse{
		ID: user.ID, Username: user.Username, Email: user.Email, Role: user.Role,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
