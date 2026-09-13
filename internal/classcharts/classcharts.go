// Package classcharts is the project's own copy of the per-class level
// progression charts and of the equipment rows that are not printed in any
// sourcebook's parseable text.
//
// These numbers are printed only as images in the class compendium, so no
// parser produces them. They were originally bootstrapped from the community
// Mastersheet xlsx; that dependency is gone — the data now lives here, in
// this repo, as reviewable JSON with no external file to read. The values are
// the effective ones as shipped (the old loader wrote some of them into
// *_override columns because the sheet had no column for them at all; here
// they are simply the value).
//
// Editing: these are hand-maintained. Change the JSON, re-run
// `n5e-ingest charts -db out/rules.db`, and commit both. A row's "manual"
// status marks a value that no printed table carries (Puppet Master's
// upgrade-tier chart is the bulk of them) — keep it, it is what stops a
// re-ingest from treating the row as stale.
package classcharts

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed classcharts.json
var classchartsJSON []byte

//go:embed equipment.json
var equipmentJSON []byte

// Resource is one charted per-level resource cell, e.g. "Superiority Dice"
// = "1d6" at level 3. Value is text because plenty of them are dice.
type Resource struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Status string `json:"status"`
}

// Level is one row of a class's progression chart.
type Level struct {
	Level            int        `json:"level"`
	ProficiencyBonus *int       `json:"proficiency_bonus"`
	JutsuKnown       *int       `json:"jutsu_known"`
	HighestRankKnown *string    `json:"highest_rank_known"`
	FeaturesText     *string    `json:"features_text"`
	Resources        []Resource `json:"resources"`
}

// Class is one class's full 20-level chart.
type Class struct {
	Slug   string  `json:"slug"`
	Levels []Level `json:"levels"`
}

// Item is an equipment row carried here rather than parsed from a book —
// the clan/class-specific weapons and armor that appear in no printed
// price table.
type Item struct {
	Slug            string   `json:"slug"`
	Name            string   `json:"name"`
	Kind            string   `json:"kind"`
	DetectionStatus string   `json:"detection_status"`
	CostRyo         *float64 `json:"cost_ryo,omitempty"`
	Bulk            *float64 `json:"bulk,omitempty"`
	DamageDice      *string  `json:"damage_dice,omitempty"`
	DamageType      *string  `json:"damage_type,omitempty"`
	Properties      *string  `json:"properties,omitempty"`
	WeaponCategory  *string  `json:"weapon_category,omitempty"`
	ACBonus         *int     `json:"ac_bonus,omitempty"`
	ArmorCategory   *string  `json:"armor_category,omitempty"`
	ArmorAbility1   *string  `json:"armor_ability_1,omitempty"`
	ArmorAbility2   *string  `json:"armor_ability_2,omitempty"`
	ArmorMaxMod     *float64 `json:"armor_max_mod,omitempty"`
	StrengthReq     *int     `json:"strength_req,omitempty"`
}

// Classes returns every class chart. The embedded JSON is part of the
// binary, so a parse failure is a build-time mistake, not a runtime
// condition — it is reported rather than tolerated.
func Classes() ([]Class, error) {
	var doc struct {
		Classes []Class `json:"classes"`
	}
	if err := json.Unmarshal(classchartsJSON, &doc); err != nil {
		return nil, fmt.Errorf("parsing classcharts.json: %w", err)
	}
	return doc.Classes, nil
}

// Equipment returns the curated equipment rows.
func Equipment() ([]Item, error) {
	var doc struct {
		Equipment []Item `json:"equipment"`
	}
	if err := json.Unmarshal(equipmentJSON, &doc); err != nil {
		return nil, fmt.Errorf("parsing equipment.json: %w", err)
	}
	return doc.Equipment, nil
}
