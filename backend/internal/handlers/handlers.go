package handlers

import (
	"github.com/gorilla/sessions"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"

	"github.com/davidyoung/fantasy-sports/backend/internal/config"
)

// Handler holds the shared dependencies that every HTTP handler needs.
// Adding a field here makes it available to all handler methods via h.field.
type Handler struct {
	db          *pgxpool.Pool
	sessions    sessions.Store
	oauthConfig *oauth2.Config
	config      config.Config
}

func New(db *pgxpool.Pool, store sessions.Store, oauthConfig *oauth2.Config, cfg config.Config) *Handler {
	return &Handler{
		db:          db,
		sessions:    store,
		oauthConfig: oauthConfig,
		config:      cfg,
	}
}
