package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/davidyoung/fantasy-sports/backend/internal/models"
)

func (h *Handler) ListPlayers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(), "SELECT id, name, sport, position, external_id FROM players ORDER BY id")
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	players := []models.Player{}
	for rows.Next() {
		var p models.Player
		if err := rows.Scan(&p.ID, &p.Name, &p.Sport, &p.Position, &p.ExternalID); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		players = append(players, p)
	}
	respondJSON(w, http.StatusOK, players)
}

func (h *Handler) CreatePlayer(w http.ResponseWriter, r *http.Request) {
	var p models.Player
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	err := h.db.QueryRow(r.Context(),
		"INSERT INTO players (name, sport, position, external_id) VALUES ($1, $2, $3, $4) RETURNING id",
		p.Name, p.Sport, p.Position, p.ExternalID,
	).Scan(&p.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, p)
}

func (h *Handler) GetPlayer(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var p models.Player
	err = h.db.QueryRow(r.Context(),
		"SELECT id, name, sport, position, external_id FROM players WHERE id = $1", id,
	).Scan(&p.ID, &p.Name, &p.Sport, &p.Position, &p.ExternalID)
	if err != nil {
		respondError(w, http.StatusNotFound, "not found")
		return
	}
	respondJSON(w, http.StatusOK, p)
}
