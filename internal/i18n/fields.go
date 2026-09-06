// Package i18n provides runtime localization: which rules.db content fields
// are translatable, reading/writing their translations, and the UI string
// catalog used by templates and Go-generated messages.
package i18n

// TranslatableFields lists, per content table, which columns hold
// translatable prose. Columns not listed here are either structural
// (foreign keys, enums, numbers) or machine-parsed at runtime and must
// never be translated at the data level -- see the jutsu "components"
// exclusion below and internal/i18n/glossary.md's technical note.
var TranslatableFields = map[string][]string{
	"jutsu": {
		"name", "casting_time", "range", "duration", "cost_text",
		"keywords", "description", "at_higher_ranks", "category_group",
	},
	"feats": {
		"name", "prerequisites", "description",
	},
	"class_features": {
		"name", "description",
	},
	"clans": {
		// name is deliberately excluded: clan names are untranslated
		// Japanese proper nouns (glossary.md's "Clan names" section),
		// same treatment as rank titles like "Kage."
		"epithet", "overview", "extra_language", "ability_increase_text",
	},
	"clan_traits":   {"name", "description"},
	"clan_features": {"name", "description"},
	"armor_properties": {
		"name", "description",
	},
	"weapon_properties": {
		"name", "description",
	},
	"equipment": {
		// properties and damage_type are deliberately excluded: both are
		// machine-parsed (see glossary.md's technical note) -- properties
		// resolves through armor_properties/weapon_properties, damage_type
		// through the UI catalog's fixed enum.
		"name", "description",
	},
	"backgrounds": {
		"name", "description", "feature_name", "feature_text", "asi_text",
		"equipment_text", "equipment_pack_text",
	},
	"subclasses":        {"name", "description"},
	"subclass_groups":   {"display_name", "description"},
	"subclass_features": {"name", "description"},
	"class_options": {
		"name", "prerequisites", "description",
		"stat_block_ac_formula_text", "stat_block_hp_formula_text",
		"stat_block_saving_throws_text", "stat_block_resistances",
		"stat_block_immunities", "stat_block_condition_immunities",
		"stat_block_senses", "stat_block_traits_attacks_text",
	},
	"class_option_entries": {"name", "description"},
	"adversary_roles":      {"name", "description"},
	"adversary_role_traits": {
		"name", "description",
	},
	"poisons":        {"name", "description"},
	"trap_templates": {"name", "description", "vs_ability", "time_to_build", "toolkit_required"},
	"summon_tribes": {
		"name", "description", "defensive_ability", "saving_throws", "skills",
		"senses", "jutsu_save_dc_text", "jutsu_attack_bonus_text", "jutsu_specialty_text",
	},
	"summon_tribe_roles":    {"role_name", "description"},
	"summon_tribe_attacks":  {"name", "description"},
	"summon_tribe_features": {"name", "description"},
	"puppet_tool_stat_block": {
		"proficiency_rule_text", "traits_text", "saving_throws_text",
		"damage_resistance_text", "damage_immunity_text",
		"condition_immunity_text", "weapon_proficiency_text",
	},
	"puppet_armor_chassis":          {"name", "description", "properties_text"},
	"puppet_armor_chassis_property": {"name", "description"},
	"puppet_swarm_stat_block":       {"raw_text"},
	"titan_unit_card":               {"raw_text"},
	// jutsu.components is deliberately excluded: cmd/n5e/jutsu_filters.go
	// parses it at runtime (componentCodes/componentNames) to power the
	// jutsu filter chips. Translating it per-row would break filtering
	// across every jutsu. The 6 fixed labels it maps to are translated
	// through the UI catalog instead (see locales/*.json's "component."
	// keys), not through content_translations.
}

// IsTranslatable reports whether field is a recognized translatable column
// of table.
func IsTranslatable(table, field string) bool {
	for _, f := range TranslatableFields[table] {
		if f == field {
			return true
		}
	}
	return false
}

// KeyColumns lists, for tables whose natural per-row identity isn't a
// single "slug" column, which column(s) uniquely identify a row -- either
// because the PK is composite (clan_traits' (clan_slug, name)) or because
// the PK column is named something other than "slug" (puppet_tool_stat_block's
// class_slug, one row per class). Tables not listed here use their own
// "slug" column directly. Joined with KeySeparator to form the value stored
// in content_translations.slug.
var KeyColumns = map[string][]string{
	"clan_traits":             {"clan_slug", "name"},
	"summon_tribe_roles":      {"tribe_slug", "role_name"},
	"summon_tribe_attacks":    {"tribe_slug", "name"},
	"puppet_tool_stat_block":  {"class_slug"},
	"puppet_swarm_stat_block": {"class_slug"},
	"titan_unit_card":         {"class_slug"},
}

// KeySeparator joins multi-column keys (see KeyColumns) into the single
// string stored as content_translations.slug. "::" can't appear in any
// slug/name column in practice (they're either kebab-case slugs or plain
// display names), so a naive join is unambiguous enough here.
const KeySeparator = "::"

// KeyColumnsFor returns the column(s) that identify a row in table --
// KeyColumns' override if one exists, otherwise the single-column default
// used by every other content table.
func KeyColumnsFor(table string) []string {
	if cols, ok := KeyColumns[table]; ok {
		return cols
	}
	return []string{"slug"}
}
