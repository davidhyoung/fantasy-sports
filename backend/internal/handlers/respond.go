package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/davidyoung/fantasy-sports/backend/internal/models"
)

// respondJSON writes a JSON response with the given status code.
func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError writes a JSON error response: {"error": "msg"}.
func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}

// parseID extracts a named URL parameter and parses it as int64.
func parseID(r *http.Request, param string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, param), 10, 64)
}

// requireUser extracts the authenticated user from the request context.
// This must only be called in routes protected by the RequireAuth middleware.
func requireUser(r *http.Request) *models.User {
	return r.Context().Value(models.UserContextKey).(*models.User)
}
