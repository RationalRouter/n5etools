// Package bingobookdata is the project's own transcription of the Bingo Book
// (book/bingo-book): its NPC template stat blocks, General Traits glossary
// and Specialist Jutsu Lists.
//
// The published PDF is a compressed copy whose pages are JPEG images with an
// invisible OCR text layer. Every text extractor reads that OCR layer, which
// misreads digits and letters throughout (1/l, 0/O, B/8, E/£) — silently, in
// exactly the numeric fields a stat block is made of. Nothing parsed from the
// file's text is trustworthy, so this data was transcribed by reading the
// rendered page images and is loaded as-is; extracted text never reaches the
// database.
//
// SourceSHA256 is the hash of the exact file transcribed. The ingest compares
// a downloaded copy against it: a different hash means the book was revised
// and this transcription needs updating, which is reported rather than
// guessed at.
//
// Editing: hand-maintained. Change the JSON, keep this package's tests
// passing, and re-run `n5e-ingest bingobook -db out/rules.db [<pdf>]`.
package bingobookdata

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
)

// SourceSHA256 is the SHA-256 of the PDF this transcription was made from
// (Bingo Book Beta 3.11, Drive file 14vx9CGOshIjd74K5EbIz1nzFbtUb_r1D).
const SourceSHA256 = "275fbb3f89379cfff5899839ee6d19b03dab29ec0fee8fef3d27f6911e0ebb7a"

// SourceVersion is the book's own printed version string.
const SourceVersion = "3.11"

// statblocks/ holds one JSON array per book page ("p018.json"), so a page
// can be re-checked against its image on its own.
//
//go:embed statblocks/*.json
var statblocksFS embed.FS

//go:embed general_traits.json
var generalTraitsJSON []byte

//go:embed specialist_lists.json
var specialistListsJSON []byte

// Ability is one ability's score, modifier and saving-throw bonus. A block
// that prints no Saving Throws entry for an ability saves at its modifier.
type Ability struct {
	Score int `json:"score"`
	Mod   int `json:"mod"`
	Save  int `json:"save"`
}

// Skill is one entry of a block's Skills line. Stars counts the book's ★
// markers in front of it.
type Skill struct {
	Name  string `json:"name"`
	Mod   int    `json:"mod"`
	Stars int    `json:"stars,omitempty"`
}

// Entry is one named paragraph within a section. Bulleted sub-items are
// joined into Text with "•" separators, the shape the app's description
// formatter renders as a list. An empty Name is the section's lead-in.
type Entry struct {
	Name string `json:"name,omitempty"`
	Tags string `json:"tags,omitempty"`
	Text string `json:"text"`
}

// Section is one headed group of entries, in printed order.
type Section struct {
	Kind    string  `json:"kind"`
	Title   string  `json:"title"`
	Entries []Entry `json:"entries"`
}

// JutsuRank is one "<R>-Ranks (Cost: N): name, name" line.
type JutsuRank struct {
	Rank  string   `json:"rank"`
	Cost  int      `json:"cost"`
	Names []string `json:"names"`
	// New lists the names in Names printed with a "[New]" marker.
	New []string `json:"new,omitempty"`
}

// JutsuDiscipline is one discipline header and the rank lines under it.
type JutsuDiscipline struct {
	Discipline string      `json:"discipline"`
	Hit        int         `json:"hit"`
	DC         int         `json:"dc"`
	Ranks      []JutsuRank `json:"ranks"`
}

// StatBlock is one NPC template.
type StatBlock struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Page        int    `json:"page"`
	Rank        string `json:"rank"`
	Size        string `json:"size"`
	Clan        string `json:"clan"`
	ClanSlug    string `json:"clan_slug"`
	Role        string `json:"role"`
	Class       string `json:"class"`
	Affiliation string `json:"affiliation"`
	Prof        int    `json:"prof"`
	Level       int    `json:"level"`
	XP          int    `json:"xp"`

	TenacityDie   string `json:"tenacity_die,omitempty"`
	TenacityCount int    `json:"tenacity_count,omitempty"`

	AC         int    `json:"ac"`
	ACNote     string `json:"ac_note,omitempty"`
	DR         int    `json:"dr"`
	Initiative int    `json:"initiative"`
	Speed      string `json:"speed"`
	HP         string `json:"hp"` // as printed; phased blocks read "360/216/108"
	CP         int    `json:"cp"`

	Str Ability `json:"str"`
	Dex Ability `json:"dex"`
	Con Ability `json:"con"`
	Int Ability `json:"int"`
	Wis Ability `json:"wis"`
	Cha Ability `json:"cha"`

	Skills                []Skill `json:"skills"`
	DamageResistances     string  `json:"damage_resistances,omitempty"`
	DamageImmunities      string  `json:"damage_immunities,omitempty"`
	DamageVulnerabilities string  `json:"damage_vulnerabilities,omitempty"`
	ConditionResistances  string  `json:"condition_resistances,omitempty"`
	ConditionImmunities   string  `json:"condition_immunities,omitempty"`
	Senses                string  `json:"senses,omitempty"`
	PassivePerception     int     `json:"passive_perception"`

	Sections []Section         `json:"sections"`
	Jutsu    []JutsuDiscipline `json:"jutsu,omitempty"`

	// Notes records every place this transcription departs from the printed
	// page (a corrected typo'd number, a reconstructed value).
	Notes string `json:"notes,omitempty"`
	// Waive names consistency checks (see Check) this block fails as
	// printed, where the printed value is kept because the correct one
	// cannot be determined from the page. Each needs a matching Notes entry.
	Waive []string `json:"waive,omitempty"`
}

// GeneralTrait is one row of the General Traits glossary.
type GeneralTrait struct {
	Name        string `json:"name"`
	Rank        string `json:"rank"`
	Keywords    string `json:"keywords"`
	Description string `json:"description"`
	Notes       string `json:"notes,omitempty"`
}

// SpecialistList is one named Specialist Jutsu List.
type SpecialistList struct {
	Name  string   `json:"name"`
	Jutsu []string `json:"jutsu"`
}

// SectionKinds are the allowed Section.Kind values, mirroring the
// adversary_template_entries CHECK constraint.
var SectionKinds = map[string]bool{
	"traits": true, "passives": true, "actions": true, "bonus_actions": true,
	"reactions": true, "elite": true, "other": true,
}

// StatBlocks returns every transcribed stat block in book order.
func StatBlocks() ([]StatBlock, error) {
	files, err := statblocksFS.ReadDir("statblocks")
	if err != nil {
		return nil, fmt.Errorf("bingobookdata: %w", err)
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.Name())
	}
	sort.Strings(names)
	var out []StatBlock
	for _, name := range names {
		raw, err := statblocksFS.ReadFile("statblocks/" + name)
		if err != nil {
			return nil, fmt.Errorf("bingobookdata: %w", err)
		}
		var page []StatBlock
		if err := decodeStrict(raw, &page); err != nil {
			return nil, fmt.Errorf("bingobookdata: statblocks/%s: %w", name, err)
		}
		out = append(out, page...)
	}
	return out, nil
}

// GeneralTraits returns the General Traits glossary in printed order.
func GeneralTraits() ([]GeneralTrait, error) {
	var out []GeneralTrait
	if err := decodeStrict(generalTraitsJSON, &out); err != nil {
		return nil, fmt.Errorf("bingobookdata: general_traits.json: %w", err)
	}
	return out, nil
}

// SpecialistLists returns the Specialist Jutsu Lists in printed order.
func SpecialistLists() ([]SpecialistList, error) {
	var out []SpecialistList
	if err := decodeStrict(specialistListsJSON, &out); err != nil {
		return nil, fmt.Errorf("bingobookdata: specialist_lists.json: %w", err)
	}
	return out, nil
}

// decodeStrict unmarshals hand-written JSON, rejecting unknown keys so a
// misspelled field name fails loudly instead of silently dropping its value.
func decodeStrict(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// Abilities returns a block's six abilities in Str..Cha order, keyed by
// their three-letter column prefix.
func (b StatBlock) Abilities() [6]struct {
	Key string
	Ability
} {
	return [6]struct {
		Key string
		Ability
	}{{"str", b.Str}, {"dex", b.Dex}, {"con", b.Con}, {"int", b.Int}, {"wis", b.Wis}, {"cha", b.Cha}}
}
