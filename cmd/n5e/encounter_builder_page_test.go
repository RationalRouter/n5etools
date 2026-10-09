package main

import (
	"database/sql"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestAdversaryBuilderPreviewSaveAndEdit builds an adversary through the
// builder's preview, saves it, edits it, and checks the stored choices.
func TestAdversaryBuilderPreviewSaveAndEdit(t *testing.T) {
	s := testServer(t)
	seedAdversaryClasses(t, s)
	if _, err := s.rulesDB.Exec(`INSERT INTO adversary_builder_sheet_traits (slug, category, category_tag, name, rank, description, sort_order, source_book) VALUES
		('adversary-builder-trait/defender/passive-1', 'passive', 'Defender', 'Passive 1', '', 'Hold the line.', 10, 'book/adversary-builder-sheet'),
		('adversary-builder-trait/defender/role-ability', 'passive', 'Defender', 'Role Ability', '', 'Shield wall.', 11, 'book/adversary-builder-sheet'),
		('adversary-builder-trait/uchiha/adversary-clan-passive', 'passive', 'Uchiha', 'Adversary Clan Passive', '', 'Clan eyes.', 12, 'book/adversary-builder-sheet'),
		('adversary-builder-trait/cursed-seal/surge', 'class_mod', 'Cursed Seal', 'Seal Surge', 'C', 'Test.', 13, 'book/adversary-builder-sheet')`); err != nil {
		t.Fatal(err)
	}
	res, err := s.charDB.Exec(`INSERT INTO encounters (name, party_size, party_level, difficulty, budget) VALUES ('Bridge', 4, 6, 'standard', 40)`)
	if err != nil {
		t.Fatal(err)
	}
	encID, _ := res.LastInsertId()
	enc := strconv.FormatInt(encID, 10)

	page := getPage(t, s, "/encounters/builder/"+enc)
	for _, want := range []string{`id="encounter-builder-form"`, `name="adv_rank"`, `name="class_mod_tag"`, `id="builder-stats"`} {
		if !strings.Contains(page, want) {
			t.Errorf("builder page lacks %s", want)
		}
	}

	form := url.Values{
		"encounter_id": {enc}, "adversary_id": {"0"},
		"adv_rank": {"C"}, "class_slug": {"adversary-rank/standard"}, "level": {"6"},
		"clan_tag": {"Uchiha"}, "class_mod_tag": {"Cursed Seal"}, "affiliation_tag": {"Leaf Shinobi"},
		"role_slug": {"adversary-builder-role/defender"}, "armor": {"heavy"},
		"ab_con": {"3"}, "save_con": {"1"}, "save_str": {"1"},
		"trait":  {"adversary-role-trait/sturdy", "adversary-builder-sheet/sharingan", "adversary-builder-trait/cursed-seal/surge"},
		"adj_hp": {"7"}, "name": {""},
	}
	w := postPopupForm(t, s, "/encounters/builder/preview", form)
	if w.Code != http.StatusOK {
		t.Fatalf("preview: status %d: %s", w.Code, w.Body.String())
	}
	live := w.Body.String()
	for _, want := range []string{`id="builder-traits"`, `id="builder-passives"`, "Hold the line.", "Clan eyes.", "Seal Surge", "Will of Fire"} {
		if !strings.Contains(live, want) {
			t.Errorf("preview lacks %q", want)
		}
	}
	if strings.Contains(live, "Shield wall.") {
		t.Error("a Standard adversary got the Role Ability passive")
	}
	if strings.Contains(live, `id="encounter-builder-form"`) {
		t.Error("preview answered the whole page instead of its live parts")
	}

	w = postPopupForm(t, s, "/encounters/"+enc+"/builder/save", form)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/encounters/"+enc {
		t.Fatalf("save: status %d, location %q: %s", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	var advID int64
	var name, rank, clan, classMod, armor, saves string
	var con sql.NullInt64
	var adjHP, tenacity int
	if err := s.charDB.QueryRow(`SELECT id, name, adv_rank, clan_tag, class_mod_tag, armor, con_mod, save_profs, adj_hp, tenacity_max
		FROM encounter_adversaries WHERE encounter_id = ?`, encID).
		Scan(&advID, &name, &rank, &clan, &classMod, &armor, &con, &saves, &adjHP, &tenacity); err != nil {
		t.Fatal(err)
	}
	if name == "" || rank != "C" || clan != "Uchiha" || classMod != "Cursed Seal" || armor != "heavy" ||
		!con.Valid || con.Int64 != 3 || saves != "str,con" || adjHP != 7 || tenacity != 0 {
		t.Errorf("stored %q %s %s %s %s con=%v saves=%q adj_hp=%d tenacity=%d", name, rank, clan, classMod, armor, con, saves, adjHP, tenacity)
	}
	var traits int
	s.charDB.QueryRow(`SELECT COUNT(*) FROM encounter_adversary_traits WHERE adversary_id = ?`, advID).Scan(&traits)
	if traits != 3 {
		t.Errorf("stored %d traits, want 3", traits)
	}

	adv := strconv.FormatInt(advID, 10)
	detail := getPage(t, s, "/encounters/"+enc)
	if !strings.Contains(detail, `/encounters/builder/`+enc+`/`+adv) {
		t.Error("roster row has no Edit in Builder link")
	}
	edit := getPage(t, s, "/encounters/builder/"+enc+"/"+adv)
	if !strings.Contains(edit, `name="adversary_id" value="`+adv+`"`) || !regexp.MustCompile(`value="Uchiha" data-description="[^"]*" selected`).MatchString(edit) {
		t.Error("edit page did not load the saved adversary")
	}
	// A blank name is saved as a generated one, which the edit page keeps.
	if strings.HasPrefix(name, "Level ") || !strings.Contains(edit, "<h2>"+name+"</h2>") || !strings.Contains(edit, `id="builder-name" name="name" value="`+name+`"`) {
		t.Errorf("edit page lost the generated name %q", name)
	}
	if w := getPage(t, s, "/encounters/builder/name?clan=Uchiha"); !strings.HasSuffix(w, " Uchiha") {
		t.Errorf("name route gave %q, want an Uchiha family name", w)
	}

	form.Set("adversary_id", adv)
	form.Set("class_slug", "adversary-rank/elite")
	form.Set("name", "Bridge Warden")
	form["trait"] = []string{"adversary-role-trait/sturdy"}
	if w := postPopupForm(t, s, "/encounters/"+enc+"/builder/save", form); w.Code != http.StatusSeeOther {
		t.Fatalf("edit save: status %d: %s", w.Code, w.Body.String())
	}
	var count int
	s.charDB.QueryRow(`SELECT COUNT(*) FROM encounter_adversaries WHERE encounter_id = ?`, encID).Scan(&count)
	s.charDB.QueryRow(`SELECT name, rank_slug, tenacity_max FROM encounter_adversaries WHERE id = ?`, advID).Scan(&name, &rank, &tenacity)
	s.charDB.QueryRow(`SELECT COUNT(*) FROM encounter_adversary_traits WHERE adversary_id = ?`, advID).Scan(&traits)
	if count != 1 || name != "Bridge Warden" || rank != "adversary-rank/elite" || tenacity != 3 || traits != 1 {
		t.Errorf("after edit: %d rows, %q %s tenacity %d, %d traits", count, name, rank, tenacity, traits)
	}

	w = postPopupForm(t, s, "/encounters/builder/preview", form)
	if !strings.Contains(w.Body.String(), "Shield wall.") {
		t.Error("an Elite adversary is missing its Role Ability passive")
	}

	if w := postPopupForm(t, s, "/encounters/999/builder/save", form); w.Code != http.StatusNotFound {
		t.Errorf("save to a missing encounter: status %d, want 404", w.Code)
	}
	form.Set("adversary_id", "999")
	if w := postPopupForm(t, s, "/encounters/"+enc+"/builder/save", form); w.Code != http.StatusNotFound {
		t.Errorf("save over a missing adversary: status %d, want 404", w.Code)
	}
}

// TestAdversaryBuilderJutsu covers step 9: the filtered list of jutsu the
// adversary can learn, saving chosen jutsu, and the roster's jutsu column.
func TestAdversaryBuilderJutsu(t *testing.T) {
	s := testServer(t)
	seedAdversaryClasses(t, s)
	if _, err := s.rulesDB.Exec(`INSERT INTO clans (slug, name) VALUES ('clan/uchiha', 'Uchiha Clan'), ('clan/hyuga', 'Hyuga Clan')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description, clan_slug) VALUES
		('jutsu/flame-bullet', 'Flame Bullet', 'Ninjutsu', 'D', '1 action', '60 feet', 'Instant', 'HS', '4 chakra', '', 'A bullet of flame.', NULL),
		('jutsu/false-surroundings', 'False Surroundings', 'Genjutsu', 'C', '1 action', '30 feet', '1 minute', 'HS', '6 chakra', '', 'The room shifts.', NULL),
		('jutsu/thousand-birds', 'Thousand Birds', 'Ninjutsu', 'A', '1 action', 'Touch', 'Instant', 'HS', '15 chakra', '', 'Too strong for C-Rank.', NULL),
		('jutsu/fireball', 'Great Fireball', 'Hijutsu', 'C', '1 action', '60 feet', 'Instant', 'HS', '6 chakra', '', 'Uchiha fire.', 'clan/uchiha'),
		('jutsu/gentle-fist', 'Gentle Fist', 'Hijutsu', 'D', '1 action', 'Touch', 'Instant', 'HS', '3 chakra', '', 'Hyuga palm.', 'clan/hyuga')`); err != nil {
		t.Fatal(err)
	}
	res, err := s.charDB.Exec(`INSERT INTO encounters (name, party_size, party_level, difficulty, budget) VALUES ('Bridge', 4, 6, 'standard', 40)`)
	if err != nil {
		t.Fatal(err)
	}
	encID, _ := res.LastInsertId()
	enc := strconv.FormatInt(encID, 10)

	form := url.Values{
		"encounter_id": {enc}, "adversary_id": {"0"},
		"adv_rank": {"C"}, "class_slug": {"adversary-rank/standard"}, "level": {"6"},
		"clan_tag": {"Uchiha"}, "affiliation_tag": {"Factionless"},
		"role_slug": {"adversary-builder-role/defender"}, "armor": {"heavy"},
		"jutsu": {"jutsu/flame-bullet"}, "name": {"Bridge Warden"},
	}
	live := postPopupForm(t, s, "/encounters/builder/preview", form).Body.String()
	pool := live[strings.Index(live, `id="builder-jutsu-pool"`):]
	for _, want := range []string{"False Surroundings", "Great Fireball", "The room shifts."} {
		if !strings.Contains(pool, want) {
			t.Errorf("jutsu pool lacks %q", want)
		}
	}
	for _, unwanted := range []string{"Thousand Birds", "Gentle Fist", "Flame Bullet"} {
		if strings.Contains(pool, unwanted) {
			t.Errorf("jutsu pool offers %q (above the Rank, another clan's, or already chosen)", unwanted)
		}
	}
	if !regexp.MustCompile(`id="builder-jutsu-jutsu/flame-bullet" name="jutsu" value="jutsu/flame-bullet" aria-label="[^"]*" checked`).MatchString(live) {
		t.Error("the chosen jutsu is not listed as chosen")
	}

	// The level, not the Rank, sets the jutsu ranks: a level 2 S-Rank
	// Standard knows D-Rank jutsu only.
	low := url.Values{}
	for k, v := range form {
		low[k] = v
	}
	low.Set("adv_rank", "S")
	low.Set("level", "2")
	low.Set("jutsu", "jutsu/fireball")
	lowLive := postPopupForm(t, s, "/encounters/builder/preview", low).Body.String()
	lowPool := lowLive[strings.Index(lowLive, `id="builder-jutsu-pool"`):]
	if strings.Contains(lowPool, "False Surroundings") || !strings.Contains(lowPool, "Flame Bullet") {
		t.Error("a level 2 adversary's pool goes above D-Rank")
	}
	if !strings.Contains(lowLive, "1 of 2 jutsu chosen, up to D-Rank.") || !strings.Contains(lowLive, "More high-rank jutsu than this level allows.") {
		t.Error("a C-Rank jutsu on a level 2 adversary is not flagged against the D-Rank quota")
	}

	form.Set("jutsu_class", "genjutsu")
	pool = postPopupForm(t, s, "/encounters/builder/preview", form).Body.String()
	if !strings.Contains(pool, "False Surroundings") || strings.Contains(pool, "Great Fireball") {
		t.Error("the classification filter did not narrow the pool")
	}
	form.Del("jutsu_class")
	form.Set("jutsu_q", "fire")
	pool = postPopupForm(t, s, "/encounters/builder/preview", form).Body.String()
	if !strings.Contains(pool, "Great Fireball") || strings.Contains(pool, "False Surroundings") {
		t.Error("the name search did not narrow the pool")
	}

	form["jutsu"] = []string{"jutsu/flame-bullet", "jutsu/fireball", "jutsu/flame-bullet"}
	if w := postPopupForm(t, s, "/encounters/"+enc+"/builder/save", form); w.Code != http.StatusSeeOther {
		t.Fatalf("save: status %d: %s", w.Code, w.Body.String())
	}
	var advID int64
	s.charDB.QueryRow(`SELECT id FROM encounter_adversaries WHERE encounter_id = ?`, encID).Scan(&advID)
	rows, err := s.charDB.Query(`SELECT jutsu_slug, jutsu_rank, cost_text FROM encounter_adversary_jutsu WHERE adversary_id = ? ORDER BY sort_order`, advID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var slug, rank, cost string
		rows.Scan(&slug, &rank, &cost)
		got = append(got, slug+" "+rank+" "+cost)
	}
	rows.Close()
	if strings.Join(got, "|") != "jutsu/flame-bullet D 4 chakra|jutsu/fireball C 6 chakra" {
		t.Errorf("stored jutsu %q", got)
	}

	detail := getPage(t, s, "/encounters/"+enc)
	if !strings.Contains(detail, "2 jutsu") || !strings.Contains(detail, "Great Fireball (C) · Hijutsu · 6 chakra") {
		t.Error("roster row lacks its jutsu column")
	}
	edit := getPage(t, s, "/encounters/builder/"+enc+"/"+strconv.FormatInt(advID, 10))
	if !strings.Contains(edit, `value="jutsu/fireball" aria-label="Add Great Fireball" checked`) || !strings.Contains(edit, "Uchiha fire.") {
		t.Error("edit page did not load the saved jutsu")
	}

	// Deleting the adversary removes its jutsu.
	tx, _ := s.charDB.Begin()
	if err := deleteAdversary(tx, encID, advID); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	var n int
	s.charDB.QueryRow(`SELECT COUNT(*) FROM encounter_adversary_jutsu WHERE adversary_id = ?`, advID).Scan(&n)
	if n != 0 {
		t.Errorf("%d jutsu rows left after deleting the adversary", n)
	}
}
