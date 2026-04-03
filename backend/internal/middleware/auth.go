// Package middleware provides reusable HTTP middleware for the fantasy sports API.
package middleware

import (
	"context"
	"net/http"

	"github.com/gorilla/sessions"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/davidyoung/fantasy-sports/backend/internal/models"
)

const sessionName = "fantasy-session"

// RequireAuth is middleware that enforces a valid login session.
//
// How it works:
//  1. Read the session cookie from the request.
//  2. Pull the user_id that we stored during the OAuth callback.
//  3. Look up that user in the database.
//  4. Attach the *models.User to the request context so downstream
//     handlers can access it via r.Context().Value(models.UserContextKey).
//
// If any step fails the middleware responds with 401 and stops the chain.
func RequireAuth(db *pgxpool.Pool, store sessions.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, err := store.Get(r, sessionName)
			// session.IsNew is true when there is no existing cookie — i.e. not logged in.
			if err != nil || session.IsNew {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// Type-assert the stored value back to int64.
			// If the key is missing or the wrong type, ok will be false.
			userID, ok := session.Values["user_id"].(int64)
			if !ok || userID == 0 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// Load the user from the database to make sure they still exist.
			var user models.User
			err = db.QueryRow(r.Context(), `
				SELECT id, yahoo_guid, display_name, email, created_at
				FROM users
				WHERE id = $1
			`, userID).Scan(&user.ID, &user.YahooGUID, &user.DisplayName, &user.Email, &user.CreatedAt)

			if err == pgx.ErrNoRows {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}

			// Attach the user to the request context and pass control to the next handler.
			ctx := context.WithValue(r.Context(), models.UserContextKey, &user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
