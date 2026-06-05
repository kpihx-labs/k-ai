package admin

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/kpihx-labs/k-ai/internal/auth"
	"github.com/kpihx-labs/k-ai/internal/config"
	"github.com/kpihx-labs/k-ai/internal/store"
)

type Handler struct {
	Store *store.Store
	JWT   *auth.JWTManager
}

func NewHandler(st *store.Store, jwt *auth.JWTManager) *Handler {
	return &Handler{Store: st, JWT: jwt}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/api/v1/providers", h.listProviders)
	mux.HandleFunc("POST /admin/api/v1/providers", h.createProvider)
	mux.HandleFunc("GET /admin/api/v1/providers/{id}", h.getProvider)
	mux.HandleFunc("PUT /admin/api/v1/providers/{id}", h.updateProvider)
	mux.HandleFunc("DELETE /admin/api/v1/providers/{id}", h.deleteProvider)

	mux.HandleFunc("GET /admin/api/v1/aliases", h.listAliases)
	mux.HandleFunc("POST /admin/api/v1/aliases", h.createAlias)
	mux.HandleFunc("PUT /admin/api/v1/aliases/{id}", h.updateAlias)
	mux.HandleFunc("DELETE /admin/api/v1/aliases/{id}", h.deleteAlias)

	mux.HandleFunc("GET /admin/api/v1/api-keys", h.listAPIKeys)
	mux.HandleFunc("POST /admin/api/v1/api-keys", h.createAPIKey)
	mux.HandleFunc("DELETE /admin/api/v1/api-keys/{id}", h.deleteAPIKey)

	mux.HandleFunc("GET /admin/api/v1/users", h.listUsers)
	mux.HandleFunc("GET /admin/api/v1/users/{id}", h.getUser)
	mux.HandleFunc("PUT /admin/api/v1/users/{id}", h.updateUser)
	mux.HandleFunc("DELETE /admin/api/v1/users/{id}", h.deleteUser)

	mux.HandleFunc("GET /admin/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}

func (h *Handler) listProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Store.ListProviders(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range rows {
		rows[i].APIKey = maskSecret(rows[i].APIKey)
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": rows})
}

func (h *Handler) getProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	row, err := h.Store.GetProvider(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if row == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	row.APIKey = maskSecret(row.APIKey)
	writeJSON(w, http.StatusOK, row)
}

func (h *Handler) createProvider(w http.ResponseWriter, r *http.Request) {
	var row store.ProviderRow
	if err := json.NewDecoder(r.Body).Decode(&row); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if row.ID == "" || row.BaseURL == "" {
		writeErr(w, http.StatusBadRequest, "id and base_url are required")
		return
	}
	if row.Name == "" {
		row.Name = row.ID
	}
	if err := h.Store.UpsertProvider(r.Context(), row); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (h *Handler) updateProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	existing, err := h.Store.GetProvider(r.Context(), id)
	if err != nil || existing == nil {
		writeErr(w, http.StatusNotFound, "provider not found")
		return
	}

	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}

	merged := *existing
	if v, ok := patch["name"]; ok {
		json.Unmarshal(v, &merged.Name)
	}
	if v, ok := patch["base_url"]; ok {
		json.Unmarshal(v, &merged.BaseURL)
	}
	if v, ok := patch["api_key"]; ok {
		var key string
		json.Unmarshal(v, &key)
		if !isMasked(key) && key != "" {
			merged.APIKey = key
		}
	}
	if v, ok := patch["enabled"]; ok {
		json.Unmarshal(v, &merged.Enabled)
	}
	if v, ok := patch["priority"]; ok {
		json.Unmarshal(v, &merged.Priority)
	}
	if v, ok := patch["models"]; ok {
		json.Unmarshal(v, &merged.Models)
	}

	if err := h.Store.UpsertProvider(r.Context(), merged); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	merged.APIKey = maskSecret(merged.APIKey)
	writeJSON(w, http.StatusOK, merged)
}

func isMasked(s string) bool {
	return len(s) >= 2 && strings.HasPrefix(s, "**")
}

func (h *Handler) deleteProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.Store.DeleteProvider(r.Context(), id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listAliases(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Store.ListAliases(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"aliases": rows})
}

func (h *Handler) createAlias(w http.ResponseWriter, r *http.Request) {
	var row store.AliasRow
	if err := json.NewDecoder(r.Body).Decode(&row); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if row.ID == "" || row.Pattern == "" || row.ProviderID == "" {
		writeErr(w, http.StatusBadRequest, "id, pattern, provider_id are required")
		return
	}
	if row.MatchType == "" {
		row.MatchType = config.MatchExact
	}
	if err := h.Store.UpsertAlias(r.Context(), row); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (h *Handler) updateAlias(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var row store.AliasRow
	if err := json.NewDecoder(r.Body).Decode(&row); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	row.ID = id
	if row.MatchType == "" {
		row.MatchType = config.MatchGlob
	}
	if err := h.Store.UpsertAlias(r.Context(), row); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *Handler) deleteAlias(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.Store.DeleteAlias(r.Context(), id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Store.ListAPIKeys(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_keys": rows})
}

type createKeyRequest struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

func (h *Handler) createAPIKey(w http.ResponseWriter, r *http.Request) {
	var req createKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	row, err := h.Store.CreateAPIKey(r.Context(), req.Name, req.Scopes)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (h *Handler) deleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.Store.DeleteAPIKey(r.Context(), id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.Store.ListUsers(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (h *Handler) getUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	user, err := h.Store.GetUserByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if user == nil {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

type updateUserRequest struct {
	Email   *string `json:"email"`
	Role    *string `json:"role"`
	Enabled *bool   `json:"enabled"`
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Role != nil && *req.Role != "admin" && *req.Role != "user" {
		writeErr(w, http.StatusBadRequest, "role must be 'admin' or 'user'")
		return
	}
	if err := h.Store.UpdateUser(r.Context(), id, req.Email, req.Role, req.Enabled); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	user, err := h.Store.GetUserByID(r.Context(), id)
	if err != nil || user == nil {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.Store.DeleteUser(r.Context(), id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(s)-4) + s[len(s)-4:]
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}
