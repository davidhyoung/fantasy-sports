package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/sessions"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/davidyoung/fantasy-sports/backend/internal/config"
	"github.com/davidyoung/fantasy-sports/backend/internal/handlers"
	appmiddleware "github.com/davidyoung/fantasy-sports/backend/internal/middleware"
	"github.com/davidyoung/fantasy-sports/backend/internal/yahoo"
)

func main() {
	// Try loading .env from the current directory first, then the project root.
	// This lets you run `make run` from backend/ while keeping .env at the repo root.
	_ = godotenv.Load()
	_ = godotenv.Load("../.env")

	// --- Database ---
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("unable to connect to database: %v", err)
	}
	defer pool.Close()

	// --- Session store ---
	// gorilla/sessions encrypts and signs the cookie using SESSION_SECRET.
	// Anyone with this secret can forge sessions, so keep it out of version control.
	sessionSecret := os.Getenv("SESSION_SECRET")
	if sessionSecret == "" {
		log.Fatal("SESSION_SECRET is required")
	}
	store := sessions.NewCookieStore([]byte(sessionSecret))
	// --- App config ---
	cfg := config.Load()

	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   cfg.SessionMaxAge,
		HttpOnly: true, // JS cannot read this cookie — protects against XSS
		SameSite: http.SameSiteLaxMode,
	}

	// --- Yahoo OAuth config ---
	oauthCfg := yahoo.NewOAuthConfig(
		os.Getenv("YAHOO_CLIENT_ID"),
		os.Getenv("YAHOO_CLIENT_SECRET"),
		os.Getenv("YAHOO_REDIRECT_URL"),
	)

	// --- Handler ---
	h := handlers.New(pool, store, oauthCfg, cfg)

	// --- Router ---
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Auth flow — these are plain browser redirects, not JSON API calls.
	// The user's browser visits these URLs directly during the OAuth dance.
	r.Get("/auth/login", h.Login)
	r.Get("/auth/callback", h.Callback)
	r.Get("/auth/logout", h.Logout)

	// Public API routes
	r.Get("/api/health", h.Health)

	r.Route("/api/leagues", func(r chi.Router) {
		r.Get("/", h.ListLeagues)
		r.Post("/", h.CreateLeague)
		r.Get("/{id}", h.GetLeague)
		r.Get("/{id}/teams", h.ListLeagueTeams)
		// Protected league sub-routes
		r.Group(func(r chi.Router) {
			r.Use(appmiddleware.RequireAuth(pool, store))
			r.Get("/{id}/scoreboard", h.GetLeagueScoreboard)
			r.Get("/{id}/standings", h.GetLeagueStandings)
			r.Get("/{id}/players", h.SearchLeaguePlayers)
			r.Get("/{id}/players/available", h.GetAvailablePlayers)
			r.Get("/{id}/draftresults", h.GetLeagueDraftResults)
			r.Get("/{id}/keepers", h.GetLeagueKeepers)
			r.Get("/{id}/keeper-rules", h.GetKeeperRules)
			r.Put("/{id}/keeper-rules", h.UpdateKeeperRules)
			r.Get("/{id}/keeper-summary", h.GetKeeperSummary)
			r.Get("/{id}/rankings", h.GetLeagueRankings)
			r.Get("/{id}/draft-values", h.GetDraftValues)
		})
	})
	r.Route("/api/players", func(r chi.Router) {
		r.Get("/", h.ListPlayers)
		r.Post("/", h.CreatePlayer)
		r.Get("/{id}", h.GetPlayer)
	})
	r.Route("/api/teams", func(r chi.Router) {
		r.Get("/{id}", h.GetTeam)
		// Protected team sub-routes
		r.Group(func(r chi.Router) {
			r.Use(appmiddleware.RequireAuth(pool, store))
			r.Get("/{id}/roster", h.GetTeamRoster)
			r.Get("/{id}/keepers", h.ListTeamKeeperWishlist)
			r.Post("/{id}/keepers/{playerKey}", h.AddKeeperWishlist)
			r.Delete("/{id}/keepers/{playerKey}", h.RemoveKeeperWishlist)
			r.Post("/{id}/keepers/submit", h.SubmitKeepers)
			r.Delete("/{id}/keepers/submit", h.UnsubmitKeepers)
		})
	})

	// Public — pre-computed NFL projections (no auth needed, no Yahoo API calls)
	r.Route("/api/projections", func(r chi.Router) {
		r.Get("/", h.ListProjections)
		r.Get("/{gsisId}", h.GetProjectionDetail)
	})

	// Public — pre-computed player grades (real-life value)
	r.Route("/api/grades", func(r chi.Router) {
		r.Get("/", h.ListGrades)
		r.Get("/{gsisId}", h.GetPlayerGrades)
	})

	// Public — NFL player detail (metadata + YoY stats + projection)
	r.Route("/api/nfl/players", func(r chi.Router) {
		r.Get("/by-yahoo/{yahooKey}", h.GetNFLPlayerByYahooID)
		r.Get("/{gsisId}", h.GetNFLPlayer)
	})

	// Protected API routes not nested under a resource prefix
	r.Group(func(r chi.Router) {
		r.Use(appmiddleware.RequireAuth(pool, store))
		r.Get("/api/auth/me", h.Me)
		r.Post("/api/sync", h.Sync)
	})

	// --- Start server ---
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("server listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
