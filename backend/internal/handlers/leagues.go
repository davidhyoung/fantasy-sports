package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/davidyoung/fantasy-sports/backend/internal/models"
)

func (h *Handler) ListLeagues(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(),
		"SELECT id, name, sport, season, COALESCE(yahoo_key, ''), COALESCE(logo_url, ''), created_at FROM leagues ORDER BY id",
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	leagues := []models.League{}
	for rows.Next() {
		var l models.League
		if err := rows.Scan(&l.ID, &l.Name, &l.Sport, &l.Season, &l.YahooKey, &l.LogoURL, &l.CreatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		leagues = append(leagues, l)
	}
	respondJSON(w, http.StatusOK, leagues)
}

func (h *Handler) CreateLeague(w http.ResponseWriter, r *http.Request) {
	var l models.League
	if err := json.NewDecoder(r.Body).Decode(&l); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	err := h.db.QueryRow(r.Context(),
		"INSERT INTO leagues (name, sport, season) VALUES ($1, $2, $3) RETURNING id, created_at",
		l.Name, l.Sport, l.Season,
	).Scan(&l.ID, &l.CreatedAt)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, l)
}

func (h *Handler) GetLeague(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var l models.League
	err = h.db.QueryRow(r.Context(),
		"SELECT id, name, sport, season, COALESCE(yahoo_key, ''), COALESCE(logo_url, ''), created_at FROM leagues WHERE id = $1", id,
	).Scan(&l.ID, &l.Name, &l.Sport, &l.Season, &l.YahooKey, &l.LogoURL, &l.CreatedAt)
	if err != nil {
		respondError(w, http.StatusNotFound, "not found")
		return
	}
	respondJSON(w, http.StatusOK, l)
}
