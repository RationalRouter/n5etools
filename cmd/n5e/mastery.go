package main

import (
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// masteryEntryRow is one Mastery pick as shown on the sheet — see migration
// 0026_mastery.sql for the book rule this backs.
type masteryEntryRow struct {
	ID   int64
	Name string
	// Rank is what actually applies right now; StoredRank is the rank the
	// player picked. They differ only when a level edit has dropped the
	// character below the rank they had (charsheet.EffectiveMasteryRank),
	// in which case the panel says so rather than silently showing a
	// smaller number than the one that was chosen.
	Rank       int
	StoredRank int
	Capped     bool
	Bonus      int
	// RankLabel is the book's own Shinobi Rank name for this rank (Chunin/
	// Jonin/Sanin or Kage) — the vocabulary the rest of the book uses when
	// it refers to a character's Mastery in a given skill.
	RankLabel string
}

// masteryData is the Core sheet's Mastery panel — every class/subclass, not
// just Puppet Master, since Mastery (Chapter 6, page 49) is a core-book
// mechanic any character can reach via the universal Ability Score
// Improvement/Feat class feature alone.
type masteryData struct {
	Cap         int
	RankOptions []int
	Entries     []masteryEntryRow
	// AvailableNames: the 21 real skills plus this character's own current
	// tool proficiencies and custom skills (character_proficiencies, kinds
	// 'tool' and 'skill') that don't already have a Mastery entry — reuses
	// existing proficiency data rather than accepting arbitrary free text,
	// same "pick from what's already real" precedent Chakra Enhanced
	// Retrofit's seal picker uses.
	AvailableNames []string
}

func (s *server) loadMasteryData(characterID int64, sheet *charsheet.Sheet) (masteryData, error) {
	var data masteryData
	data.Cap = charsheet.MasteryRankCap(sheet.Level)
	for r := 1; r <= data.Cap; r++ {
		data.RankOptions = append(data.RankOptions, r)
	}

	entries, err := charstore.ListMastery(s.charDB, characterID)
	if err != nil {
		return data, err
	}
	assigned := map[string]bool{}
	proficient := map[string]bool{}
	for _, sk := range sheet.Skills {
		proficient[sk.Name] = sk.Proficient
	}
	for _, e := range entries {
		assigned[e.SkillName] = true
		rank := charsheet.EffectiveMasteryRank(e.Rank, sheet.Level)
		data.Entries = append(data.Entries, masteryEntryRow{
			ID: e.ID, Name: e.SkillName, Rank: rank, StoredRank: e.Rank,
			Capped:    rank < e.Rank,
			Bonus:     charsheet.MasteryBonus(rank),
			RankLabel: charsheet.MasteryRankLabel(rank, proficient[e.SkillName]),
		})
	}
	sort.Slice(data.Entries, func(i, j int) bool { return data.Entries[i].Name < data.Entries[j].Name })

	names, err := s.masteryEligibleNames(characterID)
	if err != nil {
		return data, err
	}
	for _, n := range names {
		if !assigned[n] {
			data.AvailableNames = append(data.AvailableNames, n)
		}
	}
	return data, nil
}

// masteryEligibleNames: every real skill name plus this character's own
// current tool proficiencies and player-invented custom skills, sorted
// together — the candidate list a new Mastery pick can be made from.
// The book grants Mastery "with a given skill or toolkit", and this app
// splits skills across two places (the fixed 21 in charsheet.SkillAbility,
// plus anything a player typed into Tool Proficiencies & Custom Skills, see
// loadCustomSkills) — both are skills as far as the rule is concerned, and
// both roll a d20 on this sheet, so both belong here.
func (s *server) masteryEligibleNames(characterID int64) ([]string, error) {
	seen := make(map[string]bool, len(charsheet.SkillAbility)+8)
	names := make([]string, 0, len(charsheet.SkillAbility)+8)
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	for name := range charsheet.SkillAbility {
		add(name)
	}
	rows, err := s.charDB.Query(
		`SELECT DISTINCT value FROM character_proficiencies
		 WHERE character_id = ? AND kind IN ('tool', 'skill')`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		add(name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

// masteryNameEligible re-derives the same eligibility server-side —
// handleSheetMasteryAdd's own gate, the same "the tile's disabled state is
// a UI convenience, not the real enforcement" rule every other picker on
// this sheet follows.
func (s *server) masteryNameEligible(characterID int64, name string) (bool, error) {
	if _, ok := charsheet.SkillAbility[name]; ok {
		return true, nil
	}
	var exists int
	err := s.charDB.QueryRow(
		`SELECT COUNT(*) FROM character_proficiencies
		 WHERE character_id = ? AND kind IN ('tool', 'skill') AND value = ?`,
		characterID, name,
	).Scan(&exists)
	return exists > 0, err
}

// handleSheetMasteryAdd records or updates one Mastery entry (form fields
// "skill_name", "rank") — hard-rejecting a rank above the book's own
// level-gated cap (see charsheet.MasteryRankCap) or a name that isn't a
// real skill, a custom skill, or one of this character's own toolkits.
func (s *server) handleSheetMasteryAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("skill_name"))
	rank, convErr := strconv.Atoi(r.FormValue("rank"))
	if name == "" || convErr != nil || rank < 1 {
		http.Error(w, "missing or invalid Mastery selection", http.StatusBadRequest)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for mastery add:", err)
		return
	}
	if rank > charsheet.MasteryRankCap(sheet.Level) {
		http.Error(w, "rank exceeds your current Mastery cap", http.StatusBadRequest)
		return
	}
	eligible, err := s.masteryNameEligible(id, name)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("check mastery eligibility:", err)
		return
	}
	if !eligible {
		http.Error(w, "not a skill, custom skill, or toolkit you're proficient in", http.StatusBadRequest)
		return
	}
	if err := charstore.SetMastery(s.charDB, id, name, rank); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set mastery:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_mastery")
}

// handleSheetMasteryDelete removes one Mastery entry by name.
func (s *server) handleSheetMasteryDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	name := r.PathValue("name")
	if err := charstore.RemoveMastery(s.charDB, id, name); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove mastery:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_mastery")
}
