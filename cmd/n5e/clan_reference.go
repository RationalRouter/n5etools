package main

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/sergio/n5e/internal/charsheet"
)

// handleCharacterClanReference serves the "Clan Reference" popup — the
// character's own clan's full benefit catalog (traits, proficiencies,
// jutsu, bloodline latents, and level-gated features), reusing the same
// loadClanDetail query set (server.go) the standalone /clans/{slug} page
// and the creation-flow Clan step preview already share. Unlike those two
// callers, which always show a clan's complete catalog for browsing, this
// shows the complete catalog too but dims features above the character's
// current total level — a planning view, not a hidden-until-earned one,
// matching reference.go's Class Reference popup opening the same way
// (reference-popup.js's window.open, [data-reference-popup]) and using the
// same companion-reference-locked dimming instead of removing content.
func (s *server) handleCharacterClanReference(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		var exists int
		if s.charDB.QueryRow(`SELECT COUNT(*) FROM characters WHERE id = ?`, id).Scan(&exists) == nil && exists == 0 {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for clan reference popup:", err)
		return
	}
	if sheet.ClanSlug == "" {
		http.NotFound(w, r)
		return
	}

	clan, err := loadClanDetail(s.rulesDB, sheet.ClanSlug)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load clan detail for clan reference popup:", err)
		return
	}

	for i := range clan.Features {
		f := &clan.Features[i]
		f.Locked = f.Level.Valid && int(f.Level.Int64) > sheet.Level
	}

	s.render(w, "character_clan_reference.html", map[string]any{
		"Title":         sheet.Name + " — Clan Reference",
		"CharacterID":   id,
		"CharacterName": sheet.Name,
		"Clan":          clan,
	})
}
