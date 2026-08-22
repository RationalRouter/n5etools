// Package charstore is the write side of characters.db — every function
// here mutates a character's stored INPUTS (never derived math; see
// internal/charsheet's own doc for that boundary). Plain database/sql,
// explicit tx.Begin()/defer tx.Rollback()/tx.Commit() per write, per the
// character-creation plan (internal/store is a different package: the
// PDF-ingest pipeline's upsert/override-preservation machinery, and doesn't
// apply here).
//
// Every Set* function is idempotent by design: creation steps can be
// revisited and resubmitted in any order (closing the browser mid-creation
// loses nothing), so each one clears out its own previously-written rows
// (matched by source_kind/source_ref, or by a notes tag for
// character_inventory, which has no source_kind column) before writing the
// current selection — resubmitting a step replaces it instead of
// duplicating rows underneath it.
package charstore

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// CreateDraft inserts a new character row with only a name — every other
// field keeps its schema default until a later creation step fills it in.
func CreateDraft(charDB *sql.DB, name string) (int64, error) {
	res, err := charDB.Exec(`INSERT INTO characters (name) VALUES (?)`, name)
	if err != nil {
		return 0, fmt.Errorf("insert character: %w", err)
	}
	return res.LastInsertId()
}

// SetClan applies a clan choice: records clan_slug on the character row and
// grants the clan's ability-score increases + clan_proficiencies from
// rules.db as character_ability_bonuses/character_proficiencies rows tagged
// source_kind='clan'.
//
// variantIndex and picks resolve the clan's ability increases through
// ClanAbilityVariants (see clanasi.go): variantIndex selects the spread for
// the one clan that offers a choice of spreads, and picks[i] names the
// ability for slot i. Both are ignored for a clan whose increases are fixed
// — passing nil picks is the correct call there, and the fixed abilities are
// filled in from the slots themselves.
func SetClan(charDB, rulesDB *sql.DB, characterID int64, clanSlug string, variantIndex int, picks []string) error {
	grants, err := resolveClanAbilityGrants(rulesDB, clanSlug, variantIndex, picks)
	if err != nil {
		return err
	}

	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`UPDATE characters SET clan_slug = ?, updated_at = datetime('now') WHERE id = ?`,
		clanSlug, characterID,
	); err != nil {
		return fmt.Errorf("update clan_slug: %w", err)
	}
	if _, err := tx.Exec(
		`DELETE FROM character_ability_bonuses WHERE character_id = ? AND source_kind = 'clan'`, characterID,
	); err != nil {
		return fmt.Errorf("clear clan ability bonuses: %w", err)
	}
	if _, err := tx.Exec(
		`DELETE FROM character_proficiencies WHERE character_id = ? AND source_kind = 'clan'`, characterID,
	); err != nil {
		return fmt.Errorf("clear clan proficiencies: %w", err)
	}

	for _, g := range grants {
		if _, err := tx.Exec(
			`INSERT INTO character_ability_bonuses (character_id, source_kind, source_ref, ability, amount)
			 VALUES (?, 'clan', ?, ?, ?)`,
			characterID, clanSlug, g.Ability, g.Amount,
		); err != nil {
			return fmt.Errorf("insert clan ability bonus: %w", err)
		}
	}

	type prof struct{ kind, value string }
	var profs []prof

	// Read BEFORE opening the clan_proficiencies cursor below, not after:
	// a query issued while another query's rows are still open needs a
	// second pooled connection, and under an in-memory SQLite database each
	// connection is its own empty database — the second query then fails
	// with "no such table". Every rulesDB read in this function is
	// deliberately sequential for that reason.
	//
	// clans.extra_language is a real grant that lived nowhere on the sheet:
	// four clans give a language outright (Aburame's Insect-Speak, Inuzuka's
	// Dog-Speak, Hebi's Snake-Speak, Konjiki's Machine-Speak) and none of it
	// reached the Languages list, which stayed empty no matter which clan
	// was chosen. It is not in clan_proficiencies — it is its own column on
	// the clan — so it has to be read separately and folded in here.
	//
	// The column holds the language name followed by an explanation ("Dog-
	// Speak, you can speak to & understand canine creatures."); the name is
	// everything before the first comma, and only that is stored, since the
	// Languages list wants a name, not a sentence.
	var extraLanguage sql.NullString
	if err := rulesDB.QueryRow(
		`SELECT extra_language FROM clans WHERE slug = ?`, clanSlug,
	).Scan(&extraLanguage); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("query clan extra language: %w", err)
	}
	if name := ClanLanguageName(extraLanguage.String); name != "" {
		profs = append(profs, prof{kind: "language", value: name})
	}

	profRows, err := rulesDB.Query(
		`SELECT kind, value FROM clan_proficiencies WHERE clan_slug = ?`, clanSlug)
	if err != nil {
		return fmt.Errorf("query clan proficiencies: %w", err)
	}
	for profRows.Next() {
		var p prof
		if err := profRows.Scan(&p.kind, &p.value); err != nil {
			profRows.Close()
			return fmt.Errorf("scan clan proficiency: %w", err)
		}
		profs = append(profs, p)
	}
	profRows.Close()
	if err := profRows.Err(); err != nil {
		return fmt.Errorf("clan proficiency rows: %w", err)
	}
	for _, p := range profs {
		if _, err := tx.Exec(
			`INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref)
			 VALUES (?, ?, ?, 'clan', ?)`,
			characterID, p.kind, p.value, clanSlug,
		); err != nil {
			return fmt.Errorf("insert clan proficiency: %w", err)
		}
	}

	return tx.Commit()
}

// ClanLanguageName pulls the language's name out of clans.extra_language,
// which stores the name and its explanation as one string ("Insect-Speak,
// you can understand and speak to insects."). Returns "" for the clans that
// grant no extra language, which is most of them.
func ClanLanguageName(text string) string {
	name := strings.TrimSpace(text)
	if name == "" {
		return ""
	}
	if i := strings.IndexAny(name, ",."); i >= 0 {
		name = name[:i]
	}
	return strings.TrimSpace(name)
}

// numberWords maps the spelled-out counts that actually appear in the
// book's toolkit-proficiency lines. Deliberately small: this is a lookup
// for real data, not a general English number parser.
var numberWords = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6,
}

// ToolkitChoiceCount reports how many toolkits a class_proficiencies row
// with kind='tool' is really asking the player to pick, or 0 when the row
// names a specific kit and is a plain grant.
//
// The book writes these as free prose and the ingest kept the prose
// verbatim, so scout-nin's toolkit proficiency is literally the string
// "Select Any two Toolkits", intelligence-operative's is "Pick four" and
// science-nin's is "3 of your choice". Stored as-is they became
// proficiencies named after their own instructions — which is what "no
// proper drop downs" was: the choice was never presented, just recorded.
//
// The shape is always a choosing verb ("pick"/"select"/"choice"/"of your
// choice") plus a count, and every genuine grant ("Disguise Kit",
// "Weaponsmith Kit") has neither, so requiring both keeps this from
// swallowing real kit names.
func ToolkitChoiceCount(value string) int {
	v := strings.ToLower(value)
	if !strings.Contains(v, "pick") && !strings.Contains(v, "select") && !strings.Contains(v, "choice") {
		return 0
	}
	for _, field := range strings.FieldsFunc(v, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if n, ok := numberWords[field]; ok {
			return n
		}
		if n, err := strconv.Atoi(field); err == nil && n > 0 && n <= 10 {
			return n
		}
	}
	// A choosing verb with no count reads as exactly one ("One Tool Kit of
	// your choice" is caught above by the word "one"; this covers the
	// countless variants).
	return 1
}

// SetClass applies a starting-class choice: class_slug at level 1 (creation
// only ever sets one starting class — adding a second is multiclassing,
// increment 4's job), an automatic max-value level-1 HP/chakra gain (5e
// convention: level 1 doesn't roll), and every class_proficiencies grant —
// both the fixed ones (kind != 'skill_choice') and the caller-validated
// chosenSkills picked from the class's skill_choice pool. Both grants share
// one source_kind='class' tag so a class change (or a change to which
// skills were chosen) cleanly replaces the whole set in one pass.
//
// chosenToolkits are the caller-validated picks for any kind='tool' row
// that ToolkitChoiceCount flags as a choice: the placeholder row itself is
// dropped and these are written in its place, so the character ends up
// proficient with real toolkits rather than with a sentence.
func SetClass(charDB, rulesDB *sql.DB, characterID int64, classSlug string, chosenSkills, chosenToolkits []string) error {
	var hitDie, chakraDie sql.NullInt64
	if err := rulesDB.QueryRow(
		`SELECT hit_die, chakra_die FROM classes WHERE slug = ?`, classSlug,
	).Scan(&hitDie, &chakraDie); err != nil {
		return fmt.Errorf("query class dice: %w", err)
	}

	profRows, err := rulesDB.Query(
		`SELECT kind, value FROM class_proficiencies WHERE class_slug = ? AND kind != 'skill_choice'`, classSlug)
	if err != nil {
		return fmt.Errorf("query class proficiencies: %w", err)
	}
	type prof struct{ kind, value string }
	var profs []prof
	for profRows.Next() {
		var p prof
		if err := profRows.Scan(&p.kind, &p.value); err != nil {
			profRows.Close()
			return fmt.Errorf("scan class proficiency: %w", err)
		}
		// "Select Any two Toolkits" is an instruction, not a proficiency —
		// the player's answers to it arrive as chosenToolkits and are
		// written below instead.
		if p.kind == "tool" && ToolkitChoiceCount(p.value) > 0 {
			continue
		}
		profs = append(profs, p)
	}
	profRows.Close()
	if err := profRows.Err(); err != nil {
		return fmt.Errorf("class proficiency rows: %w", err)
	}

	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM character_classes WHERE character_id = ?`, characterID); err != nil {
		return fmt.Errorf("clear character classes: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM character_level_gains WHERE character_id = ?`, characterID); err != nil {
		return fmt.Errorf("clear level gains: %w", err)
	}
	if _, err := tx.Exec(
		`DELETE FROM character_proficiencies WHERE character_id = ? AND source_kind = 'class'`, characterID,
	); err != nil {
		return fmt.Errorf("clear class proficiencies: %w", err)
	}

	if _, err := tx.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, ?, 1, 0)`,
		characterID, classSlug,
	); err != nil {
		return fmt.Errorf("insert character class: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO character_level_gains (character_id, class_slug, class_level, hp_gain, chakra_gain, method)
		 VALUES (?, ?, 1, ?, ?, 'fixed')`,
		characterID, classSlug, hitDie.Int64, chakraDie.Int64,
	); err != nil {
		return fmt.Errorf("insert level-1 gains: %w", err)
	}

	for _, p := range profs {
		if _, err := tx.Exec(
			`INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref)
			 VALUES (?, ?, ?, 'class', ?)`,
			characterID, p.kind, p.value, classSlug,
		); err != nil {
			return fmt.Errorf("insert class proficiency: %w", err)
		}
	}
	for _, skill := range chosenSkills {
		if _, err := tx.Exec(
			`INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref)
			 VALUES (?, 'skill', ?, 'class', ?)`,
			characterID, skill, classSlug,
		); err != nil {
			return fmt.Errorf("insert class skill choice: %w", err)
		}
	}
	for _, kit := range chosenToolkits {
		if kit == "" {
			continue
		}
		if _, err := tx.Exec(
			`INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref)
			 VALUES (?, 'tool', ?, 'class', ?)`,
			characterID, kit, classSlug,
		); err != nil {
			return fmt.Errorf("insert class toolkit choice: %w", err)
		}
	}

	return tx.Commit()
}

// SetAbilities records the six base ability scores AS ASSIGNED (Standard
// Array / Point Buy / manual entry) — validation of which method's rules
// were followed happens in the HTTP handler, not here; this just writes
// whatever six scores it's given, matching the "trust the player, override
// escape hatch" philosophy this plan applies to Point Buy/manual entry.
func SetAbilities(charDB *sql.DB, characterID int64, str, dex, con, intel, wis, cha int) error {
	_, err := charDB.Exec(`
		UPDATE characters SET base_str = ?, base_dex = ?, base_con = ?,
		       base_int = ?, base_wis = ?, base_cha = ?, updated_at = datetime('now')
		WHERE id = ?`,
		str, dex, con, intel, wis, cha, characterID,
	)
	return err
}

// SetBackground applies a background choice: records background_slug and
// grants background_proficiencies as character_proficiencies rows tagged
// source_kind='background'. Choices already resolved out of messy prose
// rows (see the creation handler's regex split) arrive here as plain
// (kind, value) pairs, same shape as every other row.
func SetBackground(charDB *sql.DB, characterID int64, backgroundSlug string, grants []struct{ Kind, Value string }) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`UPDATE characters SET background_slug = ?, updated_at = datetime('now') WHERE id = ?`,
		backgroundSlug, characterID,
	); err != nil {
		return fmt.Errorf("update background_slug: %w", err)
	}
	if _, err := tx.Exec(
		`DELETE FROM character_proficiencies WHERE character_id = ? AND source_kind = 'background'`, characterID,
	); err != nil {
		return fmt.Errorf("clear background proficiencies: %w", err)
	}
	for _, g := range grants {
		if _, err := tx.Exec(
			`INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref)
			 VALUES (?, ?, ?, 'background', ?)`,
			characterID, g.Kind, g.Value, backgroundSlug,
		); err != nil {
			return fmt.Errorf("insert background proficiency: %w", err)
		}
	}
	return tx.Commit()
}

// EquipmentLine is one inventory row to write — either a real item (Slug
// set) or a free-text bundle (Text set, for background equipment-pack
// prose that isn't itemized anywhere in the data).
type EquipmentLine struct {
	Slug     string // equipment.slug, or "" for a free-text line
	Text     string // custom_name, used when Slug == ""
	Quantity int
}

// SetEquipment replaces every inventory line this creation step is
// responsible for. Tagged via character_inventory.notes (the only
// free-text field on that table — there's no source_kind column to key
// off, unlike every other character_* table) so re-submitting this step
// doesn't touch inventory added any other way (e.g. later manual edits on
// the sheet, once that exists).
func SetEquipment(charDB *sql.DB, characterID int64, lines []EquipmentLine) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`DELETE FROM character_inventory WHERE character_id = ? AND notes = 'creation-equipment'`, characterID,
	); err != nil {
		return fmt.Errorf("clear creation equipment: %w", err)
	}
	for _, l := range lines {
		qty := l.Quantity
		if qty <= 0 {
			qty = 1
		}
		if l.Slug != "" {
			// Two independent equipment sources (a class starting-equipment
			// group's kit/toolkit choice and a background's own pack/flat
			// equipment text, resolved separately by handleCreateEquipment
			// before both land in this same lines slice) can name the same
			// item — a player already granted an Armorsmith Kit by their
			// subclass shouldn't also get a second, separate kit row from
			// their Background. Merge into any existing row for this
			// character_id/item_slug (same "bump the quantity, don't add a
			// second row" rule AddInventoryItem already applies on the sheet
			// itself) rather than blindly inserting — this also naturally
			// merges two same-slug lines within this very call, since the
			// first one's insert is already visible to the second's lookup
			// inside this transaction.
			//
			// Scoped to notes = 'creation-equipment' (the same tag the
			// DELETE above just cleared) so this can only ever merge into a
			// row this same function created — never into a same-slug row
			// added some other way (a manual sheet edit; the character
			// sheet has no creation-status gate, so it's reachable before
			// Finish). Without that scope, revisiting and resubmitting this
			// step would keep bumping that unrelated row's quantity forever
			// on every resubmission, since the leading DELETE only clears
			// rows already tagged 'creation-equipment' and never touches it.
			res, err := tx.Exec(
				`UPDATE character_inventory SET quantity = quantity + ?
				 WHERE character_id = ? AND item_slug = ? AND notes = 'creation-equipment'`,
				qty, characterID, l.Slug,
			)
			if err != nil {
				return fmt.Errorf("merge equipment item: %w", err)
			}
			if n, err := res.RowsAffected(); err == nil && n > 0 {
				continue
			}
			if _, err := tx.Exec(
				`INSERT INTO character_inventory (character_id, item_slug, quantity, notes)
				 VALUES (?, ?, ?, 'creation-equipment')`,
				characterID, l.Slug, qty,
			); err != nil {
				return fmt.Errorf("insert equipment item: %w", err)
			}
			continue
		}
		// A free-text equipment-pack line (background prose the ingest
		// couldn't resolve to a real item) needs a real item_slug now, same
		// as every other character_inventory row — lookupCarriedItem and
		// DetailHref both key off item_slug, and a bare custom_name with no
		// slug renders as a blank, unbulked row. Same insert-then-stamp-slug
		// two-step AddCustomItem/UnpackInventoryItem already use.
		res, err := tx.Exec(`INSERT INTO custom_items (slug, name) VALUES ('', ?)`, l.Text)
		if err != nil {
			return fmt.Errorf("create custom item for equipment text line: %w", err)
		}
		ciID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("create custom item for equipment text line: %w", err)
		}
		slug := customItemSlug(l.Text, ciID)
		if _, err := tx.Exec(`UPDATE custom_items SET slug = ? WHERE id = ?`, slug, ciID); err != nil {
			return fmt.Errorf("stamp custom item slug: %w", err)
		}
		if _, err := tx.Exec(
			`INSERT INTO character_inventory (character_id, item_slug, quantity, notes)
			 VALUES (?, ?, ?, 'creation-equipment')`,
			characterID, slug, qty,
		); err != nil {
			return fmt.Errorf("insert equipment text line: %w", err)
		}
	}
	return tx.Commit()
}

// SetJutsu replaces the character's class-learned jutsu selections (source
// = 'learned' — clan/class-feature/feat grants are separate rows this
// never touches). Shared by both creation and level-up (see the plan): the
// caller passes the level jutsu was learned at.
func SetJutsu(charDB *sql.DB, characterID int64, level int, slugs []string) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`DELETE FROM character_jutsu WHERE character_id = ? AND source = 'learned'`, characterID,
	); err != nil {
		return fmt.Errorf("clear learned jutsu: %w", err)
	}
	for _, slug := range slugs {
		if _, err := tx.Exec(
			`INSERT INTO character_jutsu (character_id, jutsu_slug, learned_at_level, source)
			 VALUES (?, ?, ?, 'learned')`,
			characterID, slug, level,
		); err != nil {
			return fmt.Errorf("insert learned jutsu: %w", err)
		}
	}
	return tx.Commit()
}

// SetAmbitions replaces the character's Drive/Goal/Fear text. Empty values
// are simply omitted, not stored as blank rows.
func SetAmbitions(charDB *sql.DB, characterID int64, drive, goal, fear string) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM character_ambitions WHERE character_id = ?`, characterID); err != nil {
		return fmt.Errorf("clear ambitions: %w", err)
	}
	for kind, text := range map[string]string{"drive": drive, "goal": goal, "fear": fear} {
		if text == "" {
			continue
		}
		if _, err := tx.Exec(
			`INSERT INTO character_ambitions (character_id, kind, text) VALUES (?, ?, ?)`,
			characterID, kind, text,
		); err != nil {
			return fmt.Errorf("insert ambition: %w", err)
		}
	}
	return tx.Commit()
}

// Finish marks creation complete. Every step up to here is an ordinary
// read-then-update against the same real character_id, so this is the only
// step that isn't reversible by simply resubmitting an earlier one — see
// the plan's creation-hub note for why that's fine (nothing is destroyed;
// a completed character's steps stay editable the same way level-up
// reuses them).
func Finish(charDB *sql.DB, characterID int64) error {
	_, err := charDB.Exec(
		`UPDATE characters SET creation_status = 'complete', updated_at = datetime('now') WHERE id = ?`,
		characterID,
	)
	return err
}
