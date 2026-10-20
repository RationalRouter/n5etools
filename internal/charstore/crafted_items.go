package charstore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/RationalRouter/n5e/internal/crafting"
)

// CraftedItemPrefix namespaces crafted-item slugs in character_inventory,
// the same way "custom/" marks a custom_items row.
const CraftedItemPrefix = "crafted/"

// CraftedItem is one saved Seal Crafting recipe (crafted_items, characters
// migration 0095). Everything derived from it — rank, cost, slots — comes
// from crafting.Calculate, never from stored columns.
type CraftedItem struct {
	ID        int64
	Slug      string
	Notes     string
	Archived  bool
	CreatedAt string
	UpdatedAt string
	Recipe    crafting.Recipe
}

// IsCraftedSlug reports whether an inventory slug points at crafted_items.
func IsCraftedSlug(slug string) bool { return strings.HasPrefix(slug, CraftedItemPrefix) }

func craftedItemSlug(name string, id int64) string {
	return CraftedItemPrefix + strings.TrimPrefix(customItemSlug(name, id), "custom/")
}

// AddCraftedItem saves a new recipe and stamps its slug from the new row id.
func AddCraftedItem(charDB *sql.DB, item CraftedItem) (CraftedItem, error) {
	inputs, err := json.Marshal(item.Recipe.Crafting)
	if err != nil {
		return CraftedItem{}, err
	}
	tx, err := charDB.Begin()
	if err != nil {
		return CraftedItem{}, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`
		INSERT INTO crafted_items (slug, name, base_slug, quality, crafter_inputs_json, notes)
		VALUES ('', ?, ?, ?, ?, ?)`,
		item.Recipe.Name, item.Recipe.BaseSlug, item.Recipe.Quality, string(inputs), item.Notes)
	if err != nil {
		return CraftedItem{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return CraftedItem{}, err
	}
	item.ID = id
	item.Slug = craftedItemSlug(item.Recipe.Name, id)
	if _, err := tx.Exec(`UPDATE crafted_items SET slug = ? WHERE id = ?`, item.Slug, id); err != nil {
		return CraftedItem{}, err
	}
	if err := writeCraftedSeals(tx, id, item.Recipe.Seals); err != nil {
		return CraftedItem{}, err
	}
	return item, tx.Commit()
}

// UpdateCraftedItem overwrites a recipe in place. The slug never changes:
// every inventory row carrying the item points at it.
func UpdateCraftedItem(charDB *sql.DB, id int64, item CraftedItem) error {
	inputs, err := json.Marshal(item.Recipe.Crafting)
	if err != nil {
		return err
	}
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`
		UPDATE crafted_items
		SET name = ?, base_slug = ?, quality = ?, crafter_inputs_json = ?, notes = ?, updated_at = datetime('now')
		WHERE id = ?`,
		item.Recipe.Name, item.Recipe.BaseSlug, item.Recipe.Quality, string(inputs), item.Notes, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	if _, err := tx.Exec(`DELETE FROM crafted_item_seals WHERE crafted_item_id = ?`, id); err != nil {
		return err
	}
	if err := writeCraftedSeals(tx, id, item.Recipe.Seals); err != nil {
		return err
	}
	return tx.Commit()
}

func writeCraftedSeals(tx *sql.Tx, id int64, seals []crafting.SealPick) error {
	for i, s := range seals {
		choices := s.Choices
		if choices == nil {
			choices = []string{}
		}
		raw, err := json.Marshal(choices)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO crafted_item_seals (crafted_item_id, seal_slug, choices_json, sort_order)
			VALUES (?, ?, ?, ?)`, id, s.Slug, string(raw), i); err != nil {
			return err
		}
	}
	return nil
}

const craftedItemColumns = `id, slug, name, base_slug, quality, crafter_inputs_json, notes, archived, created_at, updated_at`

func scanCraftedItem(row interface{ Scan(...any) error }) (CraftedItem, error) {
	var (
		it     CraftedItem
		inputs string
	)
	if err := row.Scan(&it.ID, &it.Slug, &it.Recipe.Name, &it.Recipe.BaseSlug, &it.Recipe.Quality,
		&inputs, &it.Notes, &it.Archived, &it.CreatedAt, &it.UpdatedAt); err != nil {
		return CraftedItem{}, err
	}
	it.Recipe.Crafting = crafting.DefaultCrafting()
	if err := json.Unmarshal([]byte(inputs), &it.Recipe.Crafting); err != nil {
		return CraftedItem{}, fmt.Errorf("crafted item %d inputs: %w", it.ID, err)
	}
	return it, nil
}

func loadCraftedSeals(charDB *sql.DB, items []CraftedItem) error {
	if len(items) == 0 {
		return nil
	}
	index := map[int64]int{}
	ids := make([]any, len(items))
	for i, it := range items {
		index[it.ID] = i
		ids[i] = it.ID
	}
	rows, err := charDB.Query(`
		SELECT crafted_item_id, seal_slug, choices_json FROM crafted_item_seals
		WHERE crafted_item_id IN (?`+strings.Repeat(",?", len(ids)-1)+`)
		ORDER BY crafted_item_id, sort_order, id`, ids...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id      int64
			pick    crafting.SealPick
			choices string
		)
		if err := rows.Scan(&id, &pick.Slug, &choices); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(choices), &pick.Choices); err != nil {
			return fmt.Errorf("crafted item %d seal %s choices: %w", id, pick.Slug, err)
		}
		i := index[id]
		items[i].Recipe.Seals = append(items[i].Recipe.Seals, pick)
	}
	return rows.Err()
}

// GetCraftedItem loads one recipe by id.
func GetCraftedItem(charDB *sql.DB, id int64) (CraftedItem, error) {
	it, err := scanCraftedItem(charDB.QueryRow(`SELECT `+craftedItemColumns+` FROM crafted_items WHERE id = ?`, id))
	if err != nil {
		return CraftedItem{}, err
	}
	items := []CraftedItem{it}
	if err := loadCraftedSeals(charDB, items); err != nil {
		return CraftedItem{}, err
	}
	return items[0], nil
}

// GetCraftedItemBySlug loads one recipe by its crafted/ slug.
func GetCraftedItemBySlug(charDB *sql.DB, slug string) (CraftedItem, error) {
	var id int64
	if err := charDB.QueryRow(`SELECT id FROM crafted_items WHERE slug = ?`, slug).Scan(&id); err != nil {
		return CraftedItem{}, err
	}
	return GetCraftedItem(charDB, id)
}

// ListCraftedItems returns the library's active or archived recipes, by name.
func ListCraftedItems(charDB *sql.DB, archived bool) ([]CraftedItem, error) {
	rows, err := charDB.Query(`SELECT `+craftedItemColumns+` FROM crafted_items
		WHERE archived = ? ORDER BY name COLLATE NOCASE, id`, archived)
	if err != nil {
		return nil, err
	}
	var items []CraftedItem
	for rows.Next() {
		it, err := scanCraftedItem(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, loadCraftedSeals(charDB, items)
}

// DuplicateCraftedItem saves a copy of a recipe under name, unarchived.
func DuplicateCraftedItem(charDB *sql.DB, id int64, name string) (CraftedItem, error) {
	it, err := GetCraftedItem(charDB, id)
	if err != nil {
		return CraftedItem{}, err
	}
	it.Recipe.Name = name
	return AddCraftedItem(charDB, it)
}

// SetCraftedItemArchived archives or restores a recipe. Archived items leave
// the pickers but stay valid wherever they are already carried.
func SetCraftedItemArchived(charDB *sql.DB, id int64, archived bool) error {
	res, err := charDB.Exec(`UPDATE crafted_items SET archived = ?, updated_at = datetime('now') WHERE id = ?`, archived, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ResolveMechanicalSlug maps an inventory slug to the rules.db equipment
// slug its mechanics come from: a crafted item plays as its base weapon or
// armor; any other slug is returned unchanged. An unknown crafted slug
// resolves to "", which matches no equipment row.
func ResolveMechanicalSlug(charDB *sql.DB, slug string) (string, error) {
	if !IsCraftedSlug(slug) {
		return slug, nil
	}
	var base string
	err := charDB.QueryRow(`SELECT base_slug FROM crafted_items WHERE slug = ?`, slug).Scan(&base)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return base, err
}

// ErrNotEnoughRyo is returned by GiveCraftedItem when the character can't
// pay the asked price.
var ErrNotEnoughRyo = errors.New("not enough ryo")

// CharacterPurse is a character and the Ryo they carry, for the "Add to
// character" picker.
type CharacterPurse struct {
	ID   int64
	Name string
	Ryo  float64
}

// ListCharacterPurses returns every character with their Ryo, by name.
func ListCharacterPurses(charDB *sql.DB) ([]CharacterPurse, error) {
	rows, err := charDB.Query(`SELECT id, name, COALESCE(ryo, 0) FROM characters ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CharacterPurse
	for rows.Next() {
		var p CharacterPurse
		if err := rows.Scan(&p.ID, &p.Name, &p.Ryo); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GiveCraftedItem adds one of a crafted item to a character's inventory,
// paying price Ryo from their purse in the same transaction (0 for none).
// The add is refused with ErrNotEnoughRyo when the purse would go below 0.
func GiveCraftedItem(charDB *sql.DB, characterID int64, slug string, price float64) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var ryo float64
	if err := tx.QueryRow(`SELECT COALESCE(ryo, 0) FROM characters WHERE id = ?`, characterID).Scan(&ryo); err != nil {
		return err
	}
	if price > 0 {
		if ryo < price {
			return ErrNotEnoughRyo
		}
		if _, err := tx.Exec(`UPDATE characters SET ryo = ?, updated_at = datetime('now') WHERE id = ?`, ryo-price, characterID); err != nil {
			return err
		}
	}
	res, err := tx.Exec(`UPDATE character_inventory SET quantity = quantity + 1
		WHERE character_id = ? AND item_slug = ?`, characterID, slug)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := tx.Exec(`INSERT INTO character_inventory (character_id, item_slug, quantity, equipped)
			VALUES (?, ?, 1, 0)`, characterID, slug); err != nil {
			return err
		}
	}
	return tx.Commit()
}
